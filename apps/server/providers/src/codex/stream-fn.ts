import { randomUUID } from "node:crypto";
import { zodToJsonSchema } from "zod-to-json-schema";
import type { AgentMessage, AgentTool, CacheRetention, CacheStatus, TurnUsage } from "@console/types";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import type { ThinkingLevel } from "@/agent/src/types/index.js";
import { parseSse } from "@/providers/src/shared/sse-parser.js";
import { CODEX_BASE_URL, CODEX_CLIENT_VERSION, codexResponsesUrl } from "./constants.js";
import { loadCodexCredential, refreshCodexIfNeeded } from "./oauth.js";

function toolResultText(content: unknown): string {
  return typeof content === "string" ? content : JSON.stringify(content);
}

function convertInput(messages: AgentMessage[]): Array<Record<string, unknown>> {
  const input: Array<Record<string, unknown>> = [];
  for (const message of messages) {
    if (message.role === "user") {
      const content: Array<Record<string, unknown>> = [{ type: "input_text", text: message.content }];
      for (const attachment of message.attachments ?? []) {
        content.push({
          type: "input_image",
          image_url: `data:${attachment.mimeType};base64,${attachment.data}`,
          detail: "auto",
        });
      }
      input.push({ role: "user", content });
    } else if (message.role === "assistant") {
      for (const part of message.content) {
        if (part.type === "text" && part.text) {
          input.push({ role: "assistant", content: [{ type: "output_text", text: part.text }] });
        } else if (part.type === "toolCall") {
          input.push({
            type: "function_call",
            call_id: part.call.id,
            name: part.call.name,
            arguments: typeof part.call.arguments === "string" ? part.call.arguments : JSON.stringify(part.call.arguments ?? {}),
          });
        }
      }
    } else {
      for (const result of message.results) {
        input.push({
          type: "function_call_output",
          call_id: result.toolCallId,
          output: toolResultText(result.content),
        });
      }
    }
  }
  return input;
}

function convertTools(tools: AgentTool[]): Array<Record<string, unknown>> {
  return tools.map((tool) => ({
    type: "function",
    name: tool.name,
    description: tool.description,
    parameters: normalizeCodexSchema(
      zodToJsonSchema(tool.inputSchema, { target: "openApi3", $refStrategy: "none" }),
    ),
    strict: false,
  }));
}

/**
 * Codex consumes draft-2020-12 JSON Schema. zod-to-json-schema's OpenAPI
 * output still uses draft-07's boolean exclusiveMinimum/exclusiveMaximum
 * keywords, which Codex rejects with errors such as "True is not of type
 * number". This mirrors the draft upgrade performed by oh-my-pi while
 * preserving the rest of Console's tool schema.
 */
export function normalizeCodexSchema(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalizeCodexSchema);
  if (!value || typeof value !== "object") return value;

  const schema = value as Record<string, unknown>;
  const normalized: Record<string, unknown> = {};
  for (const [key, child] of Object.entries(schema)) {
    normalized[key] = normalizeCodexSchema(child);
  }

  if (normalized.exclusiveMinimum === true) {
    if (typeof normalized.minimum === "number") normalized.exclusiveMinimum = normalized.minimum;
    else delete normalized.exclusiveMinimum;
  } else if (normalized.exclusiveMinimum === false) {
    delete normalized.exclusiveMinimum;
  }
  if (normalized.exclusiveMaximum === true) {
    if (typeof normalized.maximum === "number") normalized.exclusiveMaximum = normalized.maximum;
    else delete normalized.exclusiveMaximum;
  }

  return normalized;
}

/**
 * Normalize the Codex Responses API usage block into the agent-level
 * TurnUsage. Missing usage → cacheStatus "unknown", never a forced miss.
 * When `cacheRetention: "none"` and the endpoint never reports cache
 * metadata we report `"unsupported"` — the caller opted out, so we don't
 * pretend cache is involved.
 *
 * Shape reference:
 *   { input_tokens, input_tokens_details: { cached_tokens },
 *     output_tokens, output_tokens_details: { reasoning_tokens },
 *     total_tokens }
 */
export function normalizeCodexUsage(
  usage: unknown,
  retention: CacheRetention | undefined,
): TurnUsage | undefined {
  if (!usage || typeof usage !== "object") return undefined;
  const u = usage as Record<string, unknown>;
  const input = typeof u.input_tokens === "number" ? u.input_tokens : undefined;
  const output = typeof u.output_tokens === "number" ? u.output_tokens : 0;
  const total = typeof u.total_tokens === "number"
    ? u.total_tokens
    : (input ?? 0) + output;
  const details = (u.input_tokens_details ?? {}) as Record<string, unknown>;
  const cached = typeof details.cached_tokens === "number" ? details.cached_tokens : undefined;
  const outputDetails = (u.output_tokens_details ?? {}) as Record<string, unknown>;
  const reasoning = typeof outputDetails.reasoning_tokens === "number"
    ? outputDetails.reasoning_tokens
    : undefined;

  if (input === undefined) {
    return undefined;
  }

  const hasCacheBreakdown = cached !== undefined;
  const cacheRead = hasCacheBreakdown ? cached! : 0;
  const uncached = hasCacheBreakdown ? Math.max(0, input - cached!) : input;

  let cacheStatus: CacheStatus;
  if (hasCacheBreakdown) {
    // The endpoint reported cache data — that's the truth regardless of
    // what the caller asked for. A reported miss with retention:"none"
    // still means "the endpoint saw no cache hit"; we just don't try to
    // create one next time.
    cacheStatus = cached! > 0 ? "hit" : cached === 0 ? "miss" : "unknown";
  } else if (retention === "none") {
    // Caller disabled cache controls AND the endpoint returned no cache
    // fields — that's the unsupported case.
    cacheStatus = "unsupported";
  } else {
    // Cache controls were sent but the endpoint didn't report fields
    // back. We cannot claim a miss; "unknown" is honest.
    cacheStatus = "unknown";
  }

  return {
    input: uncached,
    cacheRead,
    cacheWrite: 0,
    output,
    ...(reasoning !== undefined ? { reasoningTokens: reasoning } : {}),
    totalTokens: total,
    cacheStatus,
  };
}

/**
 * Build a Codex request body that participates in the provider's automatic
 * prompt cache. We only attach cache controls when the caller asked for it
 * — `cacheRetention: "none"` disables them, `"short"` (the default) keeps
 * the implicit in-memory cache, and `"long"` opts into the 24h retention
 * tier that the Codex Responses API exposes.
 */
/**
 * Map Console thinking level to OpenAI Codex reasoning.effort value.
 * Codex supports all 7 levels (none through max).
 */
function mapThinkingLevelToCodex(level?: ThinkingLevel): string | undefined {
  if (!level) return undefined;
  // Direct 1:1 mapping for all levels
  return level as string;
}

function buildRequestBody(
  model: { id: string },
  systemPrompt: string,
  messages: AgentMessage[],
  tools: AgentTool[],
  retention: CacheRetention | undefined,
  promptCacheKey: string,
  thinkingLevel?: ThinkingLevel,
): Record<string, unknown> {
  const body: Record<string, unknown> = {
    model: model.id,
    input: convertInput(messages),
    stream: true,
    store: false,
    ...(systemPrompt.trim() ? { instructions: systemPrompt } : {}),
    ...(tools.length > 0 ? { tools: convertTools(tools) } : {}),
  };
  if (retention !== "none") {
    body.prompt_cache_key = promptCacheKey;
    if (retention === "long") {
      body.prompt_cache_retention = "24h";
    }
  }
  // Add thinking level if specified
  const reasoningEffort = mapThinkingLevelToCodex(thinkingLevel);
  if (reasoningEffort) {
    body.reasoning = {
      type: "enabled",
      effort: reasoningEffort,
    };
  }
  return body;
}

export const codexStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
  cacheRetention,
  cacheIdentity,
  thinkingLevel,
}) {
  const credential = await refreshCodexIfNeeded(await loadCodexCredential());
  // Stable per-conversation session id: reusing the cache identity keeps the
  // Codex Responses API's automatic prompt cache warm across turns. A fresh
  // UUID per call would defeat it. The Agent rotates this identity when
  // provider/model changes; we just propagate it here.
  const sessionId = cacheIdentity?.conversationId ?? randomUUID();
  const promptCacheKey = sessionId;
  const body = buildRequestBody(model, systemPrompt, messages, tools, cacheRetention, promptCacheKey, thinkingLevel);
  const response = await fetch(codexResponsesUrl((model as { baseUrl?: string }).baseUrl ?? CODEX_BASE_URL), {
    method: "POST",
    headers: {
      Authorization: `Bearer ${credential.accessToken}`,
      "chatgpt-account-id": credential.accountId,
      "OpenAI-Beta": "responses=experimental",
      originator: "pi",
      version: CODEX_CLIENT_VERSION,
      session_id: sessionId,
      conversation_id: sessionId,
      "x-client-request-id": sessionId,
      Accept: "text/event-stream",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
    signal,
  });
  if (!response.ok) {
    throw new Error(`Codex request failed (${response.status} ${response.statusText}): ${await response.text().catch(() => "")}`);
  }

  type FunctionCallState = {
    itemId: string;
    callId: string;
    name: string;
    arguments: string;
    finalized: boolean;
    emittedArguments: boolean;
  };

  const callsByItemId = new Map<string, FunctionCallState>();
  const callsByCallId = new Map<string, FunctionCallState>();
  const pendingDeltas = new Map<string, string>();
  const pendingFinalArguments = new Map<string, { name?: string; arguments: string }>();

  const emitArguments = (state: FunctionCallState, argumentsJson: string) => {
    if (state.emittedArguments || !argumentsJson) return null;
    state.emittedArguments = true;
    return {
      type: "toolCall" as const,
      id: state.callId,
      name: state.name,
      argumentsJson,
    };
  };

  // Final cumulative usage, captured from `response.completed`. Codex sends
  // cumulative counts once at the end of the stream (not per-chunk), so we
  // attach a single `usage` delta after the stream completes — matching the
  // plan's "final cumulative record is authoritative" rule.
  let lastUsage: unknown;

  try {
    for await (const event of parseSse<Record<string, unknown>>(response)) {
      const type = typeof event.type === "string" ? event.type : "";
      if (type === "error") {
        throw new Error(String((event.error as { message?: unknown } | undefined)?.message ?? "Codex stream error"));
      }
      if (type === "response.output_text.delta" || type === "response.refusal.delta") {
        if (typeof event.delta === "string") yield { type: "text", text: event.delta };
      } else if (type === "response.reasoning_summary_text.delta" || type === "response.reasoning_text.delta") {
        if (typeof event.delta === "string") yield { type: "thinking", text: event.delta };
      } else if (type === "response.output_item.added") {
        const item = event.item as
          | { type?: string; id?: string; call_id?: string; name?: string; arguments?: string }
          | undefined;
        if (item?.type === "function_call" && item.call_id && item.name) {
          const itemId = item.id ?? item.call_id;
          const state: FunctionCallState = {
            itemId,
            callId: item.call_id,
            name: item.name,
            arguments: item.arguments ?? "",
            finalized: false,
            emittedArguments: false,
          };
          const pendingDelta = pendingDeltas.get(itemId);
          if (pendingDelta) {
            state.arguments += pendingDelta;
            pendingDeltas.delete(itemId);
          }
          const pendingFinal = pendingFinalArguments.get(itemId);
          if (pendingFinal) {
            state.name = pendingFinal.name ?? state.name;
            state.arguments = pendingFinal.arguments;
            state.finalized = true;
            pendingFinalArguments.delete(itemId);
          }

          callsByItemId.set(itemId, state);
          callsByCallId.set(state.callId, state);
          // Emit the call immediately for the UI/agent loop, but defer arguments
          // until the finalized event so streamed fragments cannot be duplicated.
          yield { type: "toolCall", id: state.callId, name: state.name, argumentsJson: "" };
          if (state.finalized) {
            const finalized = emitArguments(state, state.arguments);
            if (finalized) yield finalized;
          }
        }
      } else if (type === "response.function_call_arguments.delta") {
        const itemId = typeof event.item_id === "string" ? event.item_id : "";
        const fragment = String(event.delta ?? "");
        const state = callsByItemId.get(itemId);
        if (state) {
          state.arguments += fragment;
        } else if (itemId) {
          pendingDeltas.set(itemId, `${pendingDeltas.get(itemId) ?? ""}${fragment}`);
        }
      } else if (type === "response.function_call_arguments.done") {
        const itemId = typeof event.item_id === "string" ? event.item_id : "";
        const argumentsJson = String(event.arguments ?? "");
        const state = callsByItemId.get(itemId);
        if (state) {
          state.name = typeof event.name === "string" ? event.name : state.name;
          state.arguments = argumentsJson;
          state.finalized = true;
          const finalized = emitArguments(state, state.arguments);
          if (finalized) yield finalized;
        } else if (itemId) {
          pendingFinalArguments.set(itemId, {
            name: typeof event.name === "string" ? event.name : undefined,
            arguments: argumentsJson,
          });
        }
      } else if (type === "response.completed" || type === "response.incomplete") {
        // Capture the final usage block; the last cumulative record wins.
        const responseBlock = event.response as { usage?: unknown } | undefined;
        if (responseBlock?.usage !== undefined) {
          lastUsage = responseBlock.usage;
        }
        if (type === "response.incomplete") {
          const responseError = event.response as { error?: { message?: string } } | undefined;
          if (responseError?.error?.message) throw new Error(responseError.error.message);
        }
      } else if (type === "response.failed") {
        const responseError = event.response as { error?: { message?: string } } | undefined;
        if (responseError?.error?.message) throw new Error(responseError.error.message);
      }
    }
  } finally {
    // The finalized event is normally guaranteed, but flushing here keeps a
    // completed stream with only argument deltas from silently becoming `{}`.
    for (const state of callsByCallId.values()) {
      if (!state.finalized) {
        const assembled = emitArguments(state, state.arguments);
        if (assembled) yield assembled;
      }
    }

    // Emit the final usage delta after the stream completes. If the endpoint
    // never delivered usage (older Codex versions, malformed chunk), surface
    // cacheStatus: "unknown" rather than a forced miss.
    const usage = normalizeCodexUsage(lastUsage, cacheRetention) ?? {
      input: 0,
      cacheRead: 0,
      cacheWrite: 0,
      output: 0,
      totalTokens: 0,
      cacheStatus: cacheRetention === "none" ? ("unsupported" as const) : ("unknown" as const),
    };
    yield { type: "usage", usage };
  }
};
