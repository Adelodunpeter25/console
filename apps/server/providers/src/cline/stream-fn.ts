/**
 * Cline StreamFn — OpenAI-compatible /v1/chat/completions via the AI SDK.
 * Same wire format as OpenCode Zen. Auth: Bearer CLINE_API_KEY.
 *
 * The key is read per-call (not at module load) so the user can add it
 * mid-session. The AI SDK client is built once per call.
 */
import { streamText } from "ai";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import { CLINE_BASE_URL } from "./constants.js";
import { convertOpencodeMessages } from "@/providers/src/opencode/convert-messages.js";
import { convertOpencodeTools } from "@/providers/src/opencode/convert-tools.js";
import { loadClineCredential } from "./auth.js";

export const clineStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
}) {
  const cred = await loadClineCredential();
  if (!cred) {
    throw new Error(
      "Cline is not configured. Set CLINE_API_KEY or add a key via POST /api/auth/cline/login.",
    );
  }

  const cline = createOpenAICompatible({
    name: "cline",
    baseURL: CLINE_BASE_URL,
    apiKey: cred.apiKey,
    headers: { "X-Title": "Console" },
  });

  const convertedMessages = convertOpencodeMessages(messages);
  const convertedTools = convertOpencodeTools(tools);

  let streamError: unknown = null;

  const result = streamText({
    model: cline.chatModel(model.id),
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
      throw (part as any).error ?? new Error("Cline stream error");
    }
    if (part.type === "text-delta") {
      yield { type: "text", text: part.text };
    } else if (part.type === "reasoning-delta") {
      yield { type: "thinking", text: part.text };
    } else if (part.type === "tool-input-start") {
      yield { type: "toolCall", id: part.id, name: part.toolName, argumentsJson: "" };
    } else if (part.type === "tool-input-delta") {
      yield { type: "toolCall", id: part.id, name: "", argumentsJson: part.delta };
    }
    // "tool-call" is intentionally ignored: it carries the complete input,
    // but the agent loop already accumulated it from tool-input-start +
    // tool-input-delta fragments. Re-yielding it would duplicate the JSON.
  }

  if (streamError) {
    throw streamError;
  }
};