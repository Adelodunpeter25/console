/**
 * OpenCode Zen StreamFn — OpenAI-compatible /chat/completions via the AI SDK.
 *
 * No API key required (free tier). Emits text, thinking, and toolCall deltas.
 */
import { streamText } from "ai";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { createOpenAI } from "@ai-sdk/openai";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import { OPENCODE_BASE_URL, OPENCODE_USER_AGENT } from "./constants.js";
import { convertOpencodeMessages } from "./convert-messages.js";
import { convertOpencodeTools } from "./convert-tools.js";

const opencodeChat = createOpenAICompatible({
  name: "opencode",
  baseURL: OPENCODE_BASE_URL,
  headers: {
    "User-Agent": OPENCODE_USER_AGENT,
  },
});

const opencodeResponses = createOpenAI({
  baseURL: OPENCODE_BASE_URL,
  apiKey: "dummy",
  headers: {
    "User-Agent": OPENCODE_USER_AGENT,
  },
  fetch: (url, options) => {
    const headers = new Headers(options?.headers);
    headers.delete("authorization");
    return fetch(url, { ...options, headers });
  },
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

export const opencodeStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
}) {
  const convertedMessages = convertOpencodeMessages(messages);
  const convertedTools = convertOpencodeTools(tools);

  let streamError: unknown = null;

  const result = streamText({
    model: getOpencodeLanguageModel(model.id),
    system: systemPrompt,
    messages: convertedMessages,
    ...(Object.keys(convertedTools).length > 0 ? { tools: convertedTools } : {}),
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
};
