import { requireNativeModule, type EventSubscription } from "expo-modules-core";
import { Platform } from "react-native";
import type { AgentSessionEvent, NotificationEvent } from "@console/types";

/**
 * expo-modules-core's EventEmitter/Subscription typings drift between SDK
 * versions; this wrapper only relies on the stable runtime contract
 * (addListener -> { remove }). In SDK 52+ the native module object itself is
 * the event emitter.
 */
let emitter: {
  addListener(eventName: string, listener: (event: never) => void): EventSubscription;
} | null = null;

let NativeStreamModule: any = null;

try {
  NativeStreamModule = requireNativeModule("NativeStreamModule");
  if (NativeStreamModule) {
    // SDK 52+: the native module object is already an EventEmitter.
    emitter = NativeStreamModule;
  }
} catch {
  // Falls back gracefully in web / test environments
}

export interface StreamCallbacks {
  /**
   * One SSE frame. `meta.seq` carries the server-assigned event sequence
   * number (SSE `id:` field) when available, for re-attach resume support.
   */
  onEvent: (event: AgentSessionEvent, meta?: { seq?: number }) => void;
  /**
   * Transport failure (network drop, non-2xx, etc.). `info.statusCode`
   * carries the HTTP status when the failure came from a response.
   */
  onError: (error: string, info?: { statusCode?: number }) => void;
  /**
   * Stream finished. `aborted` is true only when the caller cancelled the
   * stream itself — a network drop reports aborted=false so consumers can
   * distinguish user intent from connection loss.
   */
  onEnd: (aborted: boolean) => void;
}

export interface NotificationCallbacks {
  onNotification: (event: NotificationEvent) => void;
  onError?: (error: string) => void;
}

export function isNativeStreamAvailable(): boolean {
  return Boolean(NativeStreamModule && emitter && Platform.OS === "android");
}

export function startNativeChatStream(
  streamId: string,
  url: string,
  body: Record<string, unknown>,
  callbacks: StreamCallbacks,
  headers: Record<string, string> = { "Content-Type": "application/json" }
): () => void {
  let finished = false;

  const emitError = (err: string, info?: { statusCode?: number }) => {
    if (finished) return;
    finished = true;
    callbacks.onError(err, info);
    // A transport failure is NOT a user abort — report aborted=false so
    // consumers can reconnect instead of treating it as intentional cancel.
    callbacks.onEnd(false);
  };

  const emitEnd = (aborted = false) => {
    if (finished) return;
    finished = true;
    callbacks.onEnd(aborted);
  };

  if (isNativeStreamAvailable()) {
    const subscriptions: EventSubscription[] = [];
    let cleanupFn: () => void = () => {};

    const eventSub = emitter!.addListener(
      "onStreamEvent",
      (data: { streamId: string; rawJson: string; seq?: string }) => {
        if (data.streamId === streamId && !finished) {
          try {
            const parsed = JSON.parse(data.rawJson) as AgentSessionEvent;
            const seq = data.seq != null ? Number.parseInt(data.seq, 10) : undefined;
            callbacks.onEvent(parsed, {
              seq: Number.isFinite(seq) ? seq : undefined,
            });
          } catch {
            // ignore malformed frame
          }
        }
      }
    );
    subscriptions.push(eventSub);

    const errorSub = emitter!.addListener(
      "onStreamError",
      (data: { streamId: string; error: string; statusCode?: number }) => {
        if (data.streamId === streamId && !finished) {
          cleanupFn();
          emitError(data.error, { statusCode: data.statusCode });
        }
      }
    );
    subscriptions.push(errorSub);

    const endSub = emitter!.addListener(
      "onStreamEnd",
      (data: { streamId: string; aborted?: boolean }) => {
        if (data.streamId === streamId && !finished) {
          cleanupFn();
          emitEnd(Boolean(data.aborted));
        }
      }
    );
    subscriptions.push(endSub);

    cleanupFn = () => {
      subscriptions.forEach((sub) => {
        try {
          sub.remove();
        } catch {
          // ignore
        }
      });
      subscriptions.length = 0;
    };

    NativeStreamModule.startChatStream(
      streamId,
      url,
      JSON.stringify(body),
      headers
    ).catch((err: Error) => {
      if (!finished) {
        cleanupFn();
        emitError(err?.message || "Failed to start native stream");
      }
    });

    return () => {
      if (!finished) {
        emitEnd(true);
        NativeStreamModule.abortStream(streamId).catch(() => {});
      }
    };
  }

  // Fallback implementation for iOS/Web or mock environments
  const xhr = new XMLHttpRequest();
  xhr.open("POST", url);
  Object.entries(headers).forEach(([k, v]) => xhr.setRequestHeader(k, v));

  let offset = 0;
  let buffer = "";

  xhr.onprogress = () => {
    if (finished || !xhr) return;
    const chunk = xhr.responseText.slice(offset);
    offset = xhr.responseText.length;
    buffer += chunk;
    const lines = buffer.split("\n");
    buffer = lines.pop() || "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed.startsWith("data: ")) continue;
      try {
        const event = JSON.parse(trimmed.slice(6)) as AgentSessionEvent;
        callbacks.onEvent(event);
      } catch {
        // ignore
      }
    }
  };

  xhr.onload = () => {
    if (finished) return;
    if (xhr.status >= 400) {
      emitError(`Server responded with status ${xhr.status}`, { statusCode: xhr.status });
    } else {
      emitEnd(false);
    }
  };

  xhr.onerror = () => {
    if (finished) return;
    emitError("Failed to connect to the backend.");
  };

  xhr.send(JSON.stringify(body));

  return () => {
    if (!finished) {
      emitEnd(true);
      try {
        xhr.abort();
      } catch {
        // ignore
      }
    }
  };
}

/**
 * Open a GET SSE stream (run re-attach). Same callback contract as
 * startNativeChatStream. Uses the native module on Android; falls back to XHR
 * elsewhere (the fallback does not surface per-frame seq).
 */
export function startNativeGetStream(
  streamId: string,
  url: string,
  callbacks: StreamCallbacks,
  headers: Record<string, string> = {}
): () => void {
  let finished = false;

  const emitError = (err: string, info?: { statusCode?: number }) => {
    if (finished) return;
    finished = true;
    callbacks.onError(err, info);
    callbacks.onEnd(false);
  };

  const emitEnd = (aborted = false) => {
    if (finished) return;
    finished = true;
    callbacks.onEnd(aborted);
  };

  if (isNativeStreamAvailable()) {
    const subscriptions: EventSubscription[] = [];

    const eventSub = emitter!.addListener(
      "onStreamEvent",
      (data: { streamId: string; rawJson: string; seq?: string }) => {
        if (data.streamId === streamId && !finished) {
          try {
            const parsed = JSON.parse(data.rawJson) as AgentSessionEvent;
            const seq = data.seq != null ? Number.parseInt(data.seq, 10) : undefined;
            callbacks.onEvent(parsed, { seq: Number.isFinite(seq) ? seq : undefined });
          } catch {
            // ignore malformed frame
          }
        }
      }
    );
    subscriptions.push(eventSub);

    const errorSub = emitter!.addListener(
      "onStreamError",
      (data: { streamId: string; error: string; statusCode?: number }) => {
        if (data.streamId === streamId && !finished) {
          cleanup();
          emitError(data.error, { statusCode: data.statusCode });
        }
      }
    );
    subscriptions.push(errorSub);

    const endSub = emitter!.addListener(
      "onStreamEnd",
      (data: { streamId: string; aborted?: boolean }) => {
        if (data.streamId === streamId && !finished) {
          cleanup();
          emitEnd(Boolean(data.aborted));
        }
      }
    );
    subscriptions.push(endSub);

    const cleanup = () => {
      subscriptions.forEach((sub) => {
        try {
          sub.remove();
        } catch {
          // ignore
        }
      });
      subscriptions.length = 0;
    };

    NativeStreamModule.startGetStream(streamId, url, headers).catch((err: Error) => {
      if (!finished) {
        cleanup();
        emitError(err?.message || "Failed to start native stream");
      }
    });

    return () => {
      if (!finished) {
        emitEnd(true);
        NativeStreamModule.abortStream(streamId).catch(() => {});
      }
    };
  }

  // Fallback implementation for iOS/Web or mock environments
  const xhr = new XMLHttpRequest();
  let offset = 0;
  let buffer = "";

  xhr.open("GET", url);
  Object.entries(headers).forEach(([k, v]) => xhr.setRequestHeader(k, v));
  xhr.setRequestHeader("Accept", "text/event-stream");

  xhr.onprogress = () => {
    if (finished || !xhr) return;
    const chunk = xhr.responseText.slice(offset);
    offset = xhr.responseText.length;
    buffer += chunk;
    const lines = buffer.split("\n");
    buffer = lines.pop() || "";

    for (const line of lines) {
      handleSseLine(line.trim(), callbacks);
    }
  };

  xhr.onload = () => {
    if (finished) return;
    if (xhr.status >= 400) {
      emitError(`Server responded with status ${xhr.status}`, { statusCode: xhr.status });
    } else {
      emitEnd(false);
    }
  };

  xhr.onerror = () => {
    if (finished) return;
    emitError("Failed to connect to the backend.");
  };

  xhr.send();

  return () => {
    if (!finished) {
      emitEnd(true);
      try {
        xhr.abort();
      } catch {
        // ignore
      }
    }
  };
}

/** Parse one SSE line and dispatch data frames to callbacks. */
function handleSseLine(trimmed: string, callbacks: StreamCallbacks): void {
  if (!trimmed.startsWith("data:")) return;
  try {
    const event = JSON.parse(trimmed.slice(5).trim()) as AgentSessionEvent;
    callbacks.onEvent(event);
  } catch {
    // ignore malformed frame
  }
}

export function startNativeNotificationStream(
  streamId: string,
  url: string,
  callbacks: NotificationCallbacks,
  headers: Record<string, string> = {}
): () => void {
  if (isNativeStreamAvailable()) {
    let cancelled = false;
    const subscriptions: EventSubscription[] = [];

    const eventSub = emitter!.addListener(
      "onNotificationEvent",
      (data: { streamId: string; rawJson: string }) => {
        if (data.streamId === streamId && !cancelled) {
          try {
            const parsed = JSON.parse(data.rawJson) as NotificationEvent;
            callbacks.onNotification(parsed);
          } catch {
            // ignore
          }
        }
      }
    );
    subscriptions.push(eventSub);

    const errorSub = emitter!.addListener(
      "onNotificationError",
      (data: { streamId: string; error: string }) => {
        if (data.streamId === streamId && !cancelled) {
          callbacks.onError?.(data.error);
        }
      }
    );
    subscriptions.push(errorSub);

    const cleanup = () => {
      subscriptions.forEach((sub) => sub.remove());
    };

    NativeStreamModule.startNotificationStream(streamId, url, headers).catch(
      (err: Error) => {
        callbacks.onError?.(err?.message || "Notification stream failed");
      }
    );

    return () => {
      cancelled = true;
      cleanup();
      NativeStreamModule.abortStream(streamId).catch(() => {});
    };
  }

  // Fallback fetch loop
  const controller = new AbortController();
  let cancelled = false;

  (async () => {
    try {
      const res = await fetch(url, {
        headers,
        signal: controller.signal,
      });
      if (!res.ok || !res.body) return;
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (!cancelled) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let idx: number;
        while ((idx = buffer.indexOf("\n")) !== -1) {
          const line = buffer.slice(0, idx).trim();
          buffer = buffer.slice(idx + 1);
          if (!line.startsWith("data: ")) continue;
          try {
            const payload = JSON.parse(line.slice(6)) as NotificationEvent;
            callbacks.onNotification(payload);
          } catch {
            // ignore
          }
        }
      }
    } catch (e) {
      if (!cancelled) {
        callbacks.onError?.(e instanceof Error ? e.message : "Stream error");
      }
    }
  })();

  return () => {
    cancelled = true;
    controller.abort();
  };
}
