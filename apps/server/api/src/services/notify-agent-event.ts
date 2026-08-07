/**
 * Maps agent lifecycle events to native notification payloads.
 *
 * Kept separate from run.service so the run loop only calls a single
 * `notifyAgentEvent(...)` per event and this file owns the copy.
 */
import type { AgentSessionEvent, NotificationEvent } from "@console/types";

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
): NotificationEvent {
  switch (event.type) {
    case "askQuestion":
      return {
        type: "notification",
        kind: "needs_attention",
        sessionId,
        title: "Needs Attention",
        body: event.request.question,
      };
    case "permissionRequest":
      return {
        type: "notification",
        kind: "needs_attention",
        sessionId,
        title: "Needs Attention",
        body: `${event.request.toolName} is requesting permission`,
      };
    case "error":
      return {
        type: "notification",
        kind: "needs_attention",
        sessionId,
        title: "Agent Error",
        body: event.error.message,
      };
  }
}

/**
 * Build the notification for a clean run completion.
 */
export function doneNotification(sessionId: string): NotificationEvent {
  return {
    type: "notification",
    kind: "done",
    sessionId,
    title: "Done",
    body: "Agent finished",
  };
}
