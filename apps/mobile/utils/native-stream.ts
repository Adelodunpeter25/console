import { NativeModules, NativeEventEmitter, Platform } from "react-native";
import type { AgentSessionEvent, NotificationEvent } from "@console/types";
import { createSseParser } from "./sse";

const { NativeStreamModule } = NativeModules;

const eventEmitter =
  Platform.OS === "android" && NativeStreamModule
    ? new NativeEventEmitter(NativeStreamModule)
    : null;

export interface StreamCallbacks {
  onEvent: (event: AgentSessionEvent) => void;
  onError: (error: string) => void;
  onEnd: (aborted: boolean) => void;
}

export interface NotificationCallbacks {
  onNotification: (event: NotificationEvent) => void;
  onError?: (error: string) => void;
}

/**
 * Start a chat run stream. Uses native Kotlin OkHttp SSE streaming on Android,
 * and falls back to XMLHttpRequest/JS parser on other platforms/environments.
 */
export function startNativeChatStream(
  streamId: string,
  url: string,
  body: Record<string, unknown>,
  callbacks: StreamCallbacks,
  headers: Record<string, string> = { "Content-Type": "application/json" }
): () => void {
  if (Platform.OS === "android" && NativeStreamModule && eventEmitter) {
    let finished = false;

    const eventSub = eventEmitter.addListener(
      "onStreamEvent",
      (data: { streamId: string; rawJson: string }) => {
        if (data.streamId === streamId && !finished) {
          try {
            const parsed = JSON.parse(data.rawJson) as AgentSessionEvent;
            callbacks.onEvent(parsed);
          } catch {
            // ignore malformed JSON frame
          }
        }
      }
    );

    const errorSub = eventEmitter.addListener(
      "onStreamError",
      (data: { streamId: string; error: string; statusCode?: number }) => {
        if (data.streamId === streamId && !finished) {
          finished = true;
          cleanup();
          callbacks.onError(data.error);
        }
      }
    );

    const endSub = eventEmitter.addListener(
      "onStreamEnd",
      (data: { streamId: string; aborted?: boolean }) => {
        if (data.streamId === streamId && !finished) {
          finished = true;
          cleanup();
          callbacks.onEnd(Boolean(data.aborted));
        }
      }
    );

    const cleanup = () => {
      eventSub.remove();
      errorSub.remove();
      endSub.remove();
    };

    NativeStreamModule.startChatStream(
      streamId,
      url,
      JSON.stringify(body),
      headers
    ).catch((err: Error) => {
      if (!finished) {
        finished = true;
        cleanup();
        callbacks.onError(err?.message || "Failed to start native stream");
      }
    });

    return () => {
      if (!finished) {
        finished = true;
        cleanup();
        NativeStreamModule.abortStream(streamId).catch(() => {});
      }
    };
  }

  // Fallback implementation for iOS/Web or when NativeModule is unavailable
  const xhr = new XMLHttpRequest();
  xhr.open("POST", url);
  Object.entries(headers).forEach(([k, v]) => xhr.setRequestHeader(k, v));

  const parser = createSseParser();
  let offset = 0;
  let hadError = false;

  xhr.onprogress = () => {
    if (!xhr) return;
    const chunk = xhr.responseText.slice(offset);
    offset = xhr.responseText.length;
    const events = parser.push(chunk);
    for (const event of events) {
      callbacks.onEvent(event);
    }
  };

  xhr.onload = () => {
    parser.flush();
    if (xhr.status >= 400) {
      hadError = true;
      callbacks.onError(`Server responded with status ${xhr.status}`);
    }
    callbacks.onEnd(hadError);
  };

  xhr.onerror = () => {
    parser.flush();
    callbacks.onError("Failed to connect to the backend.");
    callbacks.onEnd(true);
  };

  xhr.send(JSON.stringify(body));

  return () => {
    try {
      xhr.abort();
    } catch {
      // ignore
    }
  };
}

/**
 * Subscribes to a GET notification SSE stream via native Kotlin OkHttp.
 */
export function startNativeNotificationStream(
  streamId: string,
  url: string,
  callbacks: NotificationCallbacks,
  headers: Record<string, string> = {}
): () => void {
  if (Platform.OS === "android" && NativeStreamModule && eventEmitter) {
    let cancelled = false;

    const eventSub = eventEmitter.addListener(
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

    const errorSub = eventEmitter.addListener(
      "onNotificationError",
      (data: { streamId: string; error: string }) => {
        if (data.streamId === streamId && !cancelled) {
          callbacks.onError?.(data.error);
        }
      }
    );

    const cleanup = () => {
      eventSub.remove();
      errorSub.remove();
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
