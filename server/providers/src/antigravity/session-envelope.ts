/**
 * Antigravity per-session request envelope.
 *
 * Based on oh-my-pi reference implementation.
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

/**
 * Build the Antigravity request envelope (sessionId, structured requestId,
 * labels) advancing the per-conversation session state. Mirrors the real
 * `antigravity/hub` client: `requestId` is `agent/<agentId>/<ts>/<trajectoryId>/<step>`
 * and `labels.last_step_index` trails the requestId step by one.
 */
export function buildEnvelope(
  state: AntigravitySessionState,
  wireModelId: string,
): {
  labels: Record<string, string>;
  sessionId: string;
  requestId: string;
} {
  // Increment step index for this request
  state.stepIndex = (state.stepIndex ?? 0) + 1;
  const step = state.stepIndex;
  
  // Build requestId in the format: agent/<agentId>/<timestamp>/<trajectoryId>/<step>
  const requestId = `agent/${state.agentId}/${Date.now()}/${state.trajectoryId}/${step}`;
  
  // Check if this is a Claude model
  const isClaude = wireModelId.toLowerCase().includes("claude");
  
  // model_enum is optional telemetry; known values from gemini-headers.ts
  const modelEnums: Record<string, string> = {
    "gemini-3.5-flash-extra-low": "MODEL_PLACEHOLDER_M187",
    "gemini-3.5-flash-low": "MODEL_PLACEHOLDER_M20",
    "gemini-3-flash-agent": "MODEL_PLACEHOLDER_M132",
    "gemini-3.1-pro-low": "MODEL_PLACEHOLDER_M36",
    "gemini-pro-agent": "MODEL_PLACEHOLDER_M16",
  };
  const modelEnum = modelEnums[wireModelId];
  
  const labels: Record<string, string> = {
    trajectory_id: state.trajectoryId,
    last_step_index: String(step - 1), // Previous step index
    used_claude: String(isClaude),
    used_claude_conservative: String(isClaude),
  };
  
  if (modelEnum) {
    labels.model_enum = modelEnum;
  }
  
  if (state.lastExecutionId) {
    labels.last_execution_id = state.lastExecutionId;
  }

  return {
    labels,
    sessionId: state.sessionId,
    requestId,
  };
}

/** Call after each successful response to capture the responseId for the next request */
export function updateLastExecutionId(
  state: AntigravitySessionState,
  responseId: string | undefined,
): void {
  if (responseId) state.lastExecutionId = responseId;
}
