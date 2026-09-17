/**
 * Maps agent lifecycle events to native notification payloads.
 *
 * Kept separate from run.service so the run loop only calls a single
 * `notifyAgentEvent(...)` per event and this file owns the copy.
 */
import type { AgentSessionEvent, NotificationEvent } from "@console/types";

/** Max banner body length: raw questions / error stacks can be thousands of chars. */
export const NOTIFICATION_BODY_MAX = 180;
/** Max subtitle length: session titles stay single-line. */
export const NOTIFICATION_SUBTITLE_MAX = 80;

/** Collapse whitespace and truncate with an ellipsis. */
export function truncateNotificationText(value: string, max: number): string {
  const singleLine = value.replace(/\s+/g, " ").trim();
  if (singleLine.length <= max) return singleLine;
  return `${singleLine.slice(0, max - 1).trimEnd()}…`;
}

/** Optional context callers attach so banners identify *which* session fired. */
export interface NotificationContext {
  sessionTitle?: string;
  /** Short preview for done banners (e.g. last assistant text). */
  summary?: string;
}

function cleanSubtitle(sessionTitle?: string): string | undefined {
  if (!sessionTitle) return undefined;
  const trimmed = sessionTitle.replace(/\s+/g, " ").trim();
  if (!trimmed) return undefined;
  return truncateNotificationText(trimmed, NOTIFICATION_SUBTITLE_MAX);
}

/** True when an event should produce a "needs attention" notification. */
export function isAttentionEvent(
  event: AgentSessionEvent,
): event is Extract<
  AgentSessionEvent,
  { type: "askQuestion" | "permissionRequest" | "error" }
> {
  return (
    event.type === "askQuestion" ||
    event.type === "permissionRequest" ||
    event.type === "error"
  );
}

/** True when an event marks a clean run completion. */
export function isDoneEvent(event: AgentSessionEvent): event is { type: "sessionEnd" } {
  return event.type === "sessionEnd";
}

/**
 * Build the notification for a needs-attention event.
 */
export function attentionNotification(
  sessionId: string,
  event: Extract<AgentSessionEvent, { type: "askQuestion" | "permissionRequest" | "error" }>,
  ctx?: NotificationContext,
): NotificationEvent {
  const subtitle = cleanSubtitle(ctx?.sessionTitle);
  switch (event.type) {
    case "askQuestion":
      return {
        type: "notification",
        kind: "needs_attention",
        sessionId,
        title: "Needs Attention",
        ...(subtitle ? { subtitle } : {}),
        body: truncateNotificationText(event.request.question, NOTIFICATION_BODY_MAX),
      };
    case "permissionRequest":
      return {
        type: "notification",
        kind: "needs_attention",
        sessionId,
        title: "Needs Attention",
        ...(subtitle ? { subtitle } : {}),
        body: truncateNotificationText(
          `${event.request.toolName} is requesting permission`,
          NOTIFICATION_BODY_MAX,
        ),
      };
    case "error":
      return {
        type: "notification",
        kind: "needs_attention",
        sessionId,
        title: "Agent Error",
        ...(subtitle ? { subtitle } : {}),
        body: truncateNotificationText(event.error.message, NOTIFICATION_BODY_MAX),
      };
  }
}

/**
 * Build the notification for a clean run completion.
 */
export function doneNotification(
  sessionId: string,
  ctx?: NotificationContext,
): NotificationEvent {
  const subtitle = cleanSubtitle(ctx?.sessionTitle);
  const summary = ctx?.summary?.replace(/\s+/g, " ").trim();
  return {
    type: "notification",
    kind: "done",
    sessionId,
    title: "Done",
    ...(subtitle ? { subtitle } : {}),
    body: summary
      ? truncateNotificationText(summary, NOTIFICATION_BODY_MAX)
      : "Agent finished",
  };
}
