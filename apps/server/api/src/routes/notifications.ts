/**
 * Notification SSE Stream (/api/notifications/stream).
 * Fans out NotificationService pushes to connected clients (the desktop's
 * Rust layer). Same shape as GET /api/fs/watch — EventEmitter subscribe,
 * streamSSE, heartbeat.
 */
import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import type { NotificationEvent } from "@console/types";
import { notificationService } from "@/api/src/services/notification.service.js";

export const notificationRoutes = new Hono();

notificationRoutes.get("/notifications/stream", (c) => {
  return streamSSE(c, async (stream) => {
    const handler = (evt: NotificationEvent) => {
      stream.writeSSE({ event: "notification", data: JSON.stringify(evt) });
    };

    notificationService.on("notification", handler);

    stream.onAbort(() => {
      notificationService.off("notification", handler);
    });

    // Keep the connection alive with a heartbeat.
    while (!stream.aborted) {
      await stream.sleep(15000);
      await stream.writeSSE({ event: "ping", data: "" });
    }
  });
});
