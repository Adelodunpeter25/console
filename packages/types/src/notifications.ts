/**
 * Native notification payloads pushed by the backend and shown by the
 * desktop app's Rust layer (tauri-plugin-notification).
 */

export type NotificationKind = "needs_attention" | "done";

export interface NotificationEvent {
  type: "notification";
  kind: NotificationKind;
  sessionId: string;
  title: string;
  /** Optional second line (macOS subtitle / Android subtext): session title. */
  subtitle?: string;
  body: string;
}
