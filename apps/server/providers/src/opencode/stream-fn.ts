/**
 * OpenCode Zen StreamFn — OpenAI-compatible /chat/completions via the AI SDK.
 *
 * No API key required (free tier). Emits text, thinking, and toolCall deltas.
 *
 * Prompt-cache support (Step 4b of docs/plan/prompt-cache-implementation-plan.md):
 *   - For Responses-API models (Muse/GPT-5/Grok): forwards `promptCacheKey` and
 *     `promptCacheRetention` via `providerOptions.openai` so the OpenAI native
 *     provider attaches them to the request. The SDK reports cached tokens
 *     through `result.usage.inputTokenDetails.cacheReadTokens`.
 *   - For Chat Completions models: the @ai-sdk/openai-compatible provider has
 *     no native prompt-cache support, so we omit cache controls and emit a
 *     `cacheStatus: "unsupported"` usage delta (consistent with the plan's
 *     rule that unsupported providers must not claim cache participation).
 *   - `cacheRetention: "none"` from the caller disables cache controls on
 *     Responses-API models even if they would otherwise be supported.
 */
import { streamText } from "ai";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { createOpenAI } from "@ai-sdk/openai";
import type { CacheRetention, CacheStatus, TurnUsage } from "@console/types";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import {
  OPENCODE_BASE_URL,
  OPENCODE_SESSION_ID,
  OPENCODE_USER_AGENT,
} from "./constants.js";
import { convertOpencodeMessages } from "./convert-messages.js";
import { convertOpencodeTools } from "./convert-tools.js";

const opencodeChat = createOpenAICompatible({
  name: "opencode",
  baseURL: OPENCODE_BASE_URL,
  headers: {
    "User-Agent": OPENCODE_USER_AGENT,
    "x-opencode-session": OPENCODE_SESSION_ID,
  },
});

const opencodeResponses = createOpenAI({
  baseURL: OPENCODE_BASE_URL,
  apiKey: "dummy",
  headers: {
    "User-Agent": OPENCODE_USER_AGENT,
    "x-opencode-session": OPENCODE_SESSION_ID,
  },
  // Cast needed: Bun's `typeof fetch` includes a `preconnect` property that a
  // plain wrapper function can't satisfy; the AI SDK only ever calls it.
  fetch: ((url, options) => {
    const headers = new Headers(options?.headers);
    headers.delete("authorization");
    return fetch(url, { ...options, headers });
  }) as typeof fetch,
});

/**
 * OpenCode Zen routes reasoning/modern models (Muse, GPT-5, Grok) through
 * the OpenAI Responses API (/v1/responses) rather than /chat/completions.
 */
export function isOpencodeResponsesModel(modelId: string): boolean {
  return (
    modelId.startsWith("muse-") ||
    modelId.startsWith("gpt-5") ||
    modelId.startsWith("grok-")
  );
}

export function getOpencodeLanguageModel(modelId: string) {
  return isOpencodeResponsesModel(modelId)
    ? opencodeResponses.responses(modelId)
    : opencodeChat.chatModel(modelId);
}

/**
 * Normalize the AI SDK's `LanguageModelUsage` into the agent-level TurnUsage.
 *
 * Per the prompt-cache plan §4.5: the "unsupported" status is reserved for
 * providers/endpoints that don't expose cache at all (chat completions on the
 * openai-compatible adapter). For providers that *do* support cache (the
 * OpenAI Responses API), we always report what the endpoint actually told
 * us, even when the caller opted out via `cacheRetention: "none"` — the
 * endpoint may still surface cache stats, and the value the user sees should
 * match reality.
 */
export function normalizeAiSdkUsage(
  usage:
    | {
        inputTokens?: number | undefined;
        inputTokenDetails?: {
          cacheReadTokens?: number | undefined;
          cacheWriteTokens?: number | undefined;
          noCacheTokens?: number | undefined;
        };
        outputTokens?: number | undefined;
        outputTokenDetails?: { reasoningTokens?: number | undefined };
        totalTokens?: number | undefined;
      }
    | undefined,
  supportsCache: boolean,
): TurnUsage {
  const input = usage?.inputTokens;
  const cacheRead = usage?.inputTokenDetails?.cacheReadTokens ?? 0;
  const cacheWrite = usage?.inputTokenDetails?.cacheWriteTokens ?? 0;
  const output = usage?.outputTokens ?? 0;
  const reasoning = usage?.outputTokenDetails?.reasoningTokens;
  const total = usage?.totalTokens ?? (input ?? 0) + output;

  let cacheStatus: CacheStatus;
  if (!supportsCache) {
    // Provider doesn't expose cache at all — always "unsupported".
    cacheStatus = "unsupported";
  } else if (input === undefined) {
    // Endpoint returned nothing meaningful.
    cacheStatus = "unknown";
  } else {
    // The AI SDK normalizes missing `cached_tokens` to 0, so we can't tell
    // "field absent" from "explicit zero" here. The plan allows us to report
    // "miss" when the endpoint returned zero cache reads alongside a prompt
    // count — that's an honest signal.
    cacheStatus = cacheRead > 0 ? "hit" : "miss";
  }

  return {
    input: input !== undefined ? Math.max(0, input - cacheRead) : 0,
    cacheRead,
    cacheWrite,
    output,
    ...(reasoning !== undefined ? { reasoningTokens: reasoning } : {}),
    totalTokens: total,
    cacheStatus,
  };
}

/**
 * Build the `providerOptions` payload for Responses-API models. We only
 * attach cache controls when the caller asked for them and the model
 * actually supports them. The AI SDK wants `JSONObject` values
 * (`string | number | boolean | null | JSONObject | JSONArray`), so we
 * type the inner object narrowly.
 */
function buildResponsesProviderOptions(
  retention: CacheRetention | undefined,
  promptCacheKey: string | undefined,
): { openai: { promptCacheKey: string; promptCacheRetention?: "24h" } } | undefined {
  if (!promptCacheKey) return undefined;
  if (retention === "none") return undefined;
  const openai: { promptCacheKey: string; promptCacheRetention?: "24h" } = { promptCacheKey };
  if (retention === "long") {
    openai.promptCacheRetention = "24h";
  }
  return { openai };
}

export const opencodeStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
  cacheRetention,
  cacheIdentity,
}) {
  const convertedMessages = convertOpencodeMessages(messages);
  const convertedTools = convertOpencodeTools(tools);

  const supportsCache = isOpencodeResponsesModel(model.id);
  const promptCacheKey = cacheIdentity?.conversationId;
  const providerOptions = supportsCache
    ? buildResponsesProviderOptions(cacheRetention, promptCacheKey)
    : undefined;

  let streamError: unknown = null;

  const result = streamText({
    model: getOpencodeLanguageModel(model.id),
    system: systemPrompt,
    messages: convertedMessages,
    ...(Object.keys(convertedTools).length > 0 ? { tools: convertedTools } : {}),
    ...(providerOptions ? { providerOptions } : {}),
    abortSignal: signal,
    onError({ error }) {
      streamError = error;
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
    // "tool-call" is intentionally ignored: it carries the complete input,
    // but the agent loop already accumulated it from tool-input-start +
    // tool-input-delta fragments. Re-yielding it would duplicate the JSON.
  }

  if (streamError) {
    throw streamError;
  }

  // Emit the final usage delta. The AI SDK normalizes provider-specific
  // usage (including OpenAI's `cached_tokens` and `reasoning_tokens`) into
  // `result.usage` (a PromiseLike), so we await and adapt it into TurnUsage
  // before yielding the final cumulative record per the prompt-cache plan.
  const sdkUsage = await result.usage;
  const usage = normalizeAiSdkUsage(sdkUsage, supportsCache);
  yield { type: "usage", usage };
};
