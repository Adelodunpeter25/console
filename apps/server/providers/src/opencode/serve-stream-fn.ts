/**
 * OpenCode sidecar StreamFn — drives a local `opencode serve` instead of
 * calling opencode.ai/zen directly (anonymous Zen inference is gated with
 * FreeTierError; the sidecar runs as the user's own CLI credentials).
 *
 * One opencode session per Console conversation (keyed by
 * cacheIdentity.conversationId), reused across turns. The first turn sends
 * the system prompt + flattened history as context; later turns send only
 * the newest user message. opencode executes its own built-in tools
 * server-side, so this StreamFn yields text/thinking/usage only — Console
 * tools are not forwarded in v1.
 *
 * Falls back to the direct-Zen `opencodeStreamFn` when no sidecar state
 * exists (e.g. server started without `console start`).
 */
import type { AgentMessage } from "@console/types";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import {
  createServeSession,
  loadSidecarRef,
  postServePrompt,
  streamServeSession,
  type SidecarRef,
} from "./serve-client.js";
import { opencodeStreamFn } from "./stream-fn.js";

const serveSessions = new Map<string, { sessionId: string; cwd: string; modelId: string }>();

function flattenMessage(msg: AgentMessage): string | null {
  if (msg.role === "user") {
    return msg.content?.trim() ? `User: ${msg.content.trim()}` : null;
  }
  if (msg.role === "assistant") {
    const text = msg.content
      .flatMap((p) => (p.type === "text" || p.type === "thinking") && p.text ? [p.text] : [])
      .join("\n");
    return text.trim() ? `Assistant: ${text.trim()}` : null;
  }
  if (msg.role === "toolResult") {
    const text = msg.results
      .map((r) => {
        const out =
          typeof r.content === "string"
            ? r.content
            : JSON.stringify(r.content);
        return `Tool ${r.toolName || r.toolCallId}: ${out}`;
      })
      .join("\n");
    return text.trim() ? text.trim() : null;
  }
  return null;
}

function latestUserText(messages: AgentMessage[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    if (msg?.role === "user" && msg.content?.trim()) return msg.content.trim();
  }
  return "(continue)";
}

/** Exported for tests. */
export function buildServePromptText(
  systemPrompt: string,
  messages: AgentMessage[],
  isFirstTurn: boolean,
): string {
  if (!isFirstTurn) return latestUserText(messages);
  const history = messages.map(flattenMessage).filter((t): t is string => t !== null);
  const context = history.length > 0 ? `\n\nConversation so far:\n${history.join("\n\n")}` : "";
  return `${systemPrompt}${context}`;
}

export const opencodeServeStreamFn: StreamFn = async function* (params) {
  const { model, systemPrompt, messages, signal, cacheIdentity } = params;
  const ref: SidecarRef | null = await loadSidecarRef();
  if (!ref) {
    yield* opencodeStreamFn({ model, systemPrompt, messages, tools: [], signal });
    return;
  }

  const cwd = process.cwd();
  const key = cacheIdentity?.conversationId ?? `${model.id}:${Date.now()}:${Math.random()}`;
  let entry = serveSessions.get(key);
  let created = false;
  if (!entry || entry.cwd !== cwd || entry.modelId !== model.id) {
    const sessionId = await createServeSession(ref, cwd, model.id);
    entry = { sessionId, cwd, modelId: model.id };
    serveSessions.set(key, entry);
    created = true;
  }

  const text = buildServePromptText(systemPrompt, messages, created);
  try {
    yield* runServeTurn(ref, entry.sessionId, cwd, text, signal);
  } catch (error) {
    // Stale session (serve restarted and forgot it) — recreate once and retry.
    if (!/404/.test(String(error))) throw error;
    serveSessions.delete(key);
    const sessionId = await createServeSession(ref, cwd, model.id);
    serveSessions.set(key, { sessionId, cwd, modelId: model.id });
    yield* runServeTurn(ref, sessionId, cwd, text, signal);
  }
};

/**
 * One turn: open the SSE subscription first (so no delta is missed), then
 * POST the prompt, then yield the stream. Separated for the recreate-retry.
 */
async function* runServeTurn(
  ref: SidecarRef,
  sessionId: string,
  cwd: string,
  text: string,
  signal?: AbortSignal,
) {
  const stream = streamServeSession(ref, sessionId, cwd, signal);
  const pending = stream.next();
  try {
    await postServePrompt(ref, sessionId, text, cwd);
  } catch (error) {
    await stream.return?.(undefined);
    throw error;
  }
  const first = await pending;
  if (!first.done && first.value) yield first.value;
  yield* stream;
};
