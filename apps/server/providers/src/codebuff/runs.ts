/**
 * Codebuff agent-run lifecycle.
 *
 * The Codebuff backend is NOT a plain OpenAI-compatible chat endpoint: every
 * chat-completions request must reference a server-registered `run_id`
 * (otherwise the API rejects it with `runId Not Found`). Mirrors the official
 * SDK's database.ts:
 *
 *   POST /api/v1/agent-runs   { action: "START", agentId }        → { runId }
 *   POST /api/v1/chat/completions  ... with codebuff_metadata.run_id
 *   POST /api/v1/agent-runs   { action: "FINISH", runId, status, … }
 */
import { CODEBUFF_API_URL } from "./constants.js";
import type { CodebuffCredential } from "./creds.js";

export interface StartRunOptions {
  credential: CodebuffCredential;
  agentId: string;
  ancestorRunIds?: string[];
  userId?: string;
}

/** POST /api/v1/agent-runs with action:START → runId (null on failure). */
export async function startAgentRun(options: StartRunOptions): Promise<string | null> {
  const { credential, agentId, ancestorRunIds = [], userId } = options;

  const response = await fetch(`${CODEBUFF_API_URL}/agent-runs`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${credential.authToken}`,
      "Content-Type": "application/json",
      ...(userId ? { "x-freebuff-acting-user-id": userId } : {}),
    },
    body: JSON.stringify({
      action: "START",
      agentId,
      ancestorRunIds,
    }),
  });

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(`Codebuff START agent run failed (${response.status}): ${detail}`);
  }

  const body = (await response.json()) as { runId?: string };
  return typeof body.runId === "string" && body.runId.length > 0 ? body.runId : null;
}

export interface FinishRunOptions {
  credential: CodebuffCredential;
  runId: string;
  status: "completed" | "failed" | "cancelled";
  totalSteps?: number;
  errorMessage?: string;
  userId?: string;
}

/** POST /api/v1/agent-runs with action: FINISH. Best-effort. */
export async function finishAgentRun(options: FinishRunOptions): Promise<void> {
  const {
    credential,
    runId,
    status,
    totalSteps = 1,
    errorMessage,
    userId,
  } = options;

  try {
    const response = await fetch(`${CODEBUFF_API_URL}/agent-runs`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${credential.authToken}`,
        "Content-Type": "application/json",
        ...(userId ? { "x-freebuff-acting-user-id": userId } : {}),
      },
      body: JSON.stringify({
        action: "FINISH",
        runId,
        status,
        totalSteps,
        directCredits: 0,
        totalCredits: 0,
        ...(errorMessage ? { errorMessage: errorMessage.slice(0, 5000) } : {}),
      }),
    });

    if (!response.ok) {
      const detail = await response.text().catch(() => "");
      throw new Error(`Codebuff finish agent run failed (${response.status}): ${detail}`);
    }
  } catch (error) {
    // FINISH is best-effort cleanup; never mask the actual stream error.
    console.error("Codebuff finishAgentRun error:", error);
  }
}