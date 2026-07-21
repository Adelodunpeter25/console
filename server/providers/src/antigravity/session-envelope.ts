/**
 * Antigravity per-session request envelope.
 *
 * Mirrors the real `antigravity/hub` client:
 *   - agentId: stable UUID for the Agent instance
 *   - trajectoryId: stable UUID for a single run()
 *   - sessionId: signed-decimal session id (stable per Agent instance)
 *   - stepIndex: monotonically increments per turn
 *   - lastExecutionId: echoed from the previous response's responseId
 */
import { randomUUID } from "node:crypto";

export interface AntigravitySessionState {
  agentId: string;
  trajectoryId: string;
  sessionId: string;
  stepIndex: number;
  lastExecutionId?: string;
}

export function createSessionState(): AntigravitySessionState {
  // sessionId mirrors what the real client uses: a large random decimal string
  const sessionId = BigInt(`0x${randomUUID().replace(/-/g, "")}`).toString();
  return {
    agentId: randomUUID(),
    trajectoryId: randomUUID(),
    sessionId,
    stepIndex: 0,
  };
}

export function buildEnvelope(
  state: AntigravitySessionState,
  wireModelId: string,
): {
  labels: Record<string, string>;
  sessionId: string;
  requestId: string;
} {
  const labels: Record<string, string> = {
    request_id: randomUUID(),
    session_id: state.sessionId,
    agent_id: state.agentId,
    trajectory_id: state.trajectoryId,
    step_index: String(state.stepIndex),
  };

  // model_enum is optional telemetry; known values from gemini-headers.ts
  const modelEnums: Record<string, string> = {
    "gemini-3.5-flash-extra-low": "MODEL_PLACEHOLDER_M187",
    "gemini-3.5-flash-low": "MODEL_PLACEHOLDER_M20",
    "gemini-3-flash-agent": "MODEL_PLACEHOLDER_M132",
    "gemini-3.1-pro-low": "MODEL_PLACEHOLDER_M36",
    "gemini-pro-agent": "MODEL_PLACEHOLDER_M16",
  };
  const modelEnum = modelEnums[wireModelId];
  if (modelEnum) labels.model_enum = modelEnum;

  if (state.lastExecutionId) {
    labels.last_execution_id = state.lastExecutionId;
  }

  state.stepIndex += 1;

  return {
    labels,
    sessionId: state.sessionId,
    requestId: labels.request_id!,
  };
}

/** Call after each successful response to capture the responseId for the next request */
export function updateLastExecutionId(
  state: AntigravitySessionState,
  responseId: string | undefined,
): void {
  if (responseId) state.lastExecutionId = responseId;
}
