import { requireNativeModule, EventEmitter, Subscription } from "expo-modules-core";
import { Platform } from "react-native";
import type { AgentSessionEvent, NotificationEvent } from "@console/types";

let NativeStreamModule: any = null;
let emitter: EventEmitter | null = null;

try {
  NativeStreamModule = requireNativeModule("NativeStreamModule");
  if (NativeStreamModule) {
    emitter = new EventEmitter(NativeStreamModule);
  }
} catch {
  // Falls back gracefully in web / test environments
}

export interface StreamCallbacks {
  onEvent: (event: AgentSessionEvent) => void;
  onError: (error: string) => void;
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
  if (isNativeStreamAvailable()) {
    let finished = false;
    const subscriptions: Subscription[] = [];

    const eventSub = emitter!.addListener(
      "onStreamEvent",
      (data: { streamId: string; rawJson: string }) => {
        if (data.streamId === streamId && !finished) {
          try {
            const parsed = JSON.parse(data.rawJson) as AgentSessionEvent;
            callbacks.onEvent(parsed);
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
          finished = true;
          cleanup();
          callbacks.onError(data.error);
        }
      }
    );
    subscriptions.push(errorSub);

    const endSub = emitter!.addListener(
      "onStreamEnd",
      (data: { streamId: string; aborted?: boolean }) => {
        if (data.streamId === streamId && !finished) {
          finished = true;
          cleanup();
          callbacks.onEnd(Boolean(data.aborted));
        }
      }
    );
    subscriptions.push(endSub);

    const cleanup = () => {
      subscriptions.forEach((sub) => sub.remove());
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

  // Fallback implementation for iOS/Web or mock environments
  const xhr = new XMLHttpRequest();
  xhr.open("POST", url);
  Object.entries(headers).forEach(([k, v]) => xhr.setRequestHeader(k, v));

  let offset = 0;
  let buffer = "";
  let hadError = false;

  xhr.onprogress = () => {
    if (!xhr) return;
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
    if (xhr.status >= 400) {
      hadError = true;
      callbacks.onError(`Server responded with status ${xhr.status}`);
    }
    callbacks.onEnd(hadError);
  };

  xhr.onerror = () => {
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

export function startNativeNotificationStream(
  streamId: string,
  url: string,
  callbacks: NotificationCallbacks,
  headers: Record<string, string> = {}
): () => void {
  if (isNativeStreamAvailable()) {
    let cancelled = false;
    const subscriptions: Subscription[] = [];

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
