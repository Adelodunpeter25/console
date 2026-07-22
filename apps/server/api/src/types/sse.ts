/**
 * SSE (Server-Sent Events) Payload Types for real-time agent execution streaming.
 */
import type { AgentSessionEvent } from "../../../agent/src/types/index.js";

export interface SseEventFrame {
  event: AgentSessionEvent["type"];
  data: AgentSessionEvent;
}
