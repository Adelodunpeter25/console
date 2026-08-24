/**
 * OpenCode Zen StreamFn — OpenAI-compatible /chat/completions via the AI SDK.
 *
 * No API key required (free tier). Emits text, thinking, and toolCall deltas.
 */
import { streamText } from "ai";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import { OPENCODE_BASE_URL } from "./constants.js";
import { convertOpencodeMessages } from "./convert-messages.js";
import { convertOpencodeTools } from "./convert-tools.js";

const opencode = createOpenAICompatible({
  name: "opencode",
  baseURL: OPENCODE_BASE_URL,
  // No apiKey — the free tier requires no Authorization header (verified live).
});

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
    model: opencode.chatModel(model.id),
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
