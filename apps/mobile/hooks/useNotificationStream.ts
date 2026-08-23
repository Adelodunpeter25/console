import { useEffect, useRef } from "react";
import type { NotificationEvent } from "@console/types";
import { useAppStore } from "@/stores/useAppStore";
import { startNativeNotificationStream } from "@/utils/native-stream";

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

    const stopStream = startNativeNotificationStream(
      `notifications-${Date.now()}`,
      `${backendUrl}/api/notifications/stream`,
      {
        onNotification: (payload) => {
          onNotificationRef.current?.(payload);
        },
      }
    );

    return () => {
      stopStream();
    };
  }, [backendUrl]);
}
