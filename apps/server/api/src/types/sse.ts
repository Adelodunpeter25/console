/**
 * SSE (Server-Sent Events) Payload Types for real-time agent execution streaming.
 */
import type { AgentSessionEvent } from "@console/types";

export interface SseEventFrame {
  event: AgentSessionEvent["type"];
  data: AgentSessionEvent;
}
