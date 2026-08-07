/**
 * Notification bus. The backend pushes notification events here and the
 * SSE route (/api/notifications/stream) fans them out to connected clients
 * (the desktop app's Rust layer, which shows native OS notifications).
 *
 * Mirrors the FsWatchService shape: a module-level EventEmitter singleton.
 */
import { EventEmitter } from "node:events";
import type { NotificationEvent } from "@console/types";

export class NotificationService extends EventEmitter {
  push(notification: NotificationEvent): void {
    this.emit("notification", notification);
  }
}

export const notificationService = new NotificationService();
