/**
 * Codebuff (Freebuff) StreamFn — OpenAI-compatible chat completions against
 * the Codebuff backend (https://www.codebuff.com/api/v1).
 *
 * IMPORTANT: the Codebuff backend is NOT a plain OpenAI-compatible endpoint.
 * Every chat-completions request must reference a server-registered `run_id`
 * (POST /api/v1/agent-runs START → runId), or the API rejects with
 * `runId Not Found`. This mirrors the official SDK's run lifecycle:
 *
 *   1. POST /api/v1/agent-runs  { action: "START", agentId }          → runId
 *   2. POST /api/v1/chat/completions  ... codebuff_metadata.run_id=runId
 *   3. POST /api/v1/agent-runs  { action: "FINISH", runId, status }   (cleanup)
 *
 * Free tier: sends `codebuff_metadata.cost_mode = "free"` and uses the
 * free-tier root agent id (`base2-free-<model>`) for the run so allowlisted
 * free-tier agent+model combos cost 0 credits.
 */
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { streamText } from "ai";

import type { StreamFn } from "../../../agent/src/service/agent-loop.js";
import { CODEBUFF_API_URL } from "./constants.js";
import { convertCodebuffMessages } from "./convert-messages.js";
import { convertCodebuffTools } from "./convert-tools.js";
import { loadCodebuffCredential } from "./creds.js";
import { ensureFreebuffSession } from "./freebuff-session.js";
import { finishAgentRun, startAgentRun } from "./runs.js";

/** Map a model id → the free-tier root agent that owns its runs (run START
 *  requires a real agentId, and free grants match base2-free-* roots). */
const FREE_AGENT_ID_BY_MODEL: Record<string, string> = {
  "deepseek/deepseek-v4-flash": "base2-free-deepseek-flash",
  "deepseek/deepseek-v4-pro": "base2-free-deepseek",
  "mimo/mimo-v2.5": "base2-free-mimo",
  "mimo/mimo-v2.5-pro": "base2-free-mimo-pro",
  "minimax/minimax-m3": "base2-free-minimax-m3",
  "moonshotai/kimi-k2.7-code": "base2-free-kimi",
};

export function resolveFreeAgentIdForModel(modelId: string): string {
  return FREE_AGENT_ID_BY_MODEL[modelId] ?? "base2-free";
}

export const codebuffStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
}) {
  const credential = await loadCodebuffCredential();
  if (!credential?.authToken) {
    throw new Error(
      "Codebuff is not logged in. Run the Codebuff login flow (POST /api/auth/codebuff/start) " +
        "or set the CODEBUFF_API_KEY environment variable.",
    );
  }

  // 0. Ensure an active freebuff session for this model. The server rejects
  //    free-mode chat requests that don't carry a live session instance id
  //    (`free_mode_cli_required`) — this is the CLI session handshake.
  const sessionResult = await ensureFreebuffSession(credential, model.id);
  if (!sessionResult.ok) {
    const { reason, message, detail } = sessionResult;
    throw new Error(
      `Codebuff free session unavailable (${reason}): ${message ?? detail ?? "no session"}`,
    );
  }
  const freebuffInstanceId = sessionResult.session.instanceId;

  // 1. Register the agent run server-side — REQUIRED, requests without a
  //    known run_id are rejected with 400 "runId Not Found".
  const agentId = resolveFreeAgentIdForModel(model.id);
  const runId = await startAgentRun({ credential, agentId });
  if (!runId) {
    throw new Error("Codebuff failed to start an agent run. Please try again.");
  }

  const codebuff = createOpenAICompatible({
    name: "codebuff",
    baseURL: CODEBUFF_API_URL,
    apiKey: credential.authToken,
  });

  const convertedMessages = convertCodebuffMessages(messages);
  const convertedTools = convertCodebuffTools(tools);

  let failed = false;
  let streamError: unknown = null;
  try {
    const result = streamText({
      model: codebuff.chatModel(model.id),
      system: systemPrompt,
      messages: convertedMessages,
      ...(Object.keys(convertedTools).length > 0 ? { tools: convertedTools } : {}),
      abortSignal: signal,
      onError({ error }) {
        streamError = error;
      },
      // Mirrors the official SDK's getProviderOptions(): codebuff_metadata
      // and provider land at the top level of the request body. run_id must
      // be the server-issued runId, and freebuff_instance_id must be the
      // live session instance the CLI would hold.
      providerOptions: {
        codebuff: {
          codebuff_metadata: {
            cost_mode: "free",
            run_id: runId,
            client_id: "console-agent",
            freebuff_instance_id: freebuffInstanceId,
          },
          provider: { allow_fallbacks: true },
        },
      },
    });

    for await (const part of result.fullStream) {
      if (part.type === "error") {
        throw (part as any).error ?? new Error("AI stream error");
      }
      if (part.type === "text-delta") {
        yield { type: "text", text: part.text };
      } else if (part.type === "reasoning-delta") {
        yield { type: "thinking", text: part.text };
      } else if (part.type === "tool-input-start") {
        yield {
          type: "toolCall",
          id: part.id,
          name: part.toolName,
          argumentsJson: "",
        };
      } else if (part.type === "tool-input-delta") {
        yield {
          type: "toolCall",
          id: part.id,
          name: "",
          argumentsJson: part.delta,
        };
      }
      // "tool-call" is intentionally ignored: the agent loop already accumulated
      // it from tool-input-start + tool-input-delta fragments.
    }

    if (streamError) {
      throw streamError;
    }
  } catch (error) {
    failed = true;
    throw error;
  } finally {
    // 3. Close the run (best-effort cleanup).
    await finishAgentRun({
      credential,
      runId,
      status: failed ? "failed" : "completed",
    });
  }
};