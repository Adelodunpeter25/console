import { useEffect, useRef } from "react";
import type { NotificationEvent } from "@console/types";
import { useAppStore } from "../stores/useAppStore";

/**
 * Subscribes to the backend's notification SSE stream
 * (GET /api/notifications/stream) and invokes the callback for each
 * notification (e.g. to show a native toast/banner).
 *
 * Mirrors the desktop's Rust notification relay, but client-side via
 * `fetch` + a reader loop (RN fetch streams responses as chunks).
 */
export function useNotificationStream(onNotification?: (event: NotificationEvent) => void) {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const onNotificationRef = useRef(onNotification);
  onNotificationRef.current = onNotification;

  useEffect(() => {
    if (!backendUrl) return;
    let cancelled = false;
    let controller: AbortController | null = null;

    const connect = async () => {
      controller = new AbortController();
      try {
        const res = await fetch(`${backendUrl}/api/notifications/stream`, {
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
              onNotificationRef.current?.(payload);
            } catch {
              // ignore malformed frames
            }
          }
        }
      } catch {
        // Abort or network error — stop silently.
      }
    };

    connect();
    return () => {
      cancelled = true;
      controller?.abort();
    };
  }, [backendUrl]);
}
