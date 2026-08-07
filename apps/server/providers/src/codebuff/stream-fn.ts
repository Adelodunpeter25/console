/**
 * Codebuff (Freebuff) StreamFn — OpenAI-compatible chat completions against
 * the Codebuff backend (https://codebuff.com/api/v1).
 *
 * Auth: `Authorization: Bearer <authToken>` where the token comes from the
 * device-code login flow (see login.ts) or the CODEBUFF_API_KEY env var.
 *
 * Free tier: sends `codebuff_metadata.cost_mode = "free"` in the request body
 * (exactly like the official SDK's getProviderOptions) so allowlisted
 * free-tier agent+model combos cost 0 credits. Premium model ids fall back to
 * the account's daily premium session pool server-side.
 */
import { randomUUID } from "node:crypto";

import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import { streamText } from "ai";

import type { StreamFn } from "../../../agent/src/service/agent-loop.js";
import { CODEBUFF_API_URL } from "./constants.js";
import { convertCodebuffMessages } from "./convert-messages.js";
import { convertCodebuffTools } from "./convert-tools.js";
import { loadCodebuffCredential } from "./creds.js";

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

  const codebuff = createOpenAICompatible({
    name: "codebuff",
    baseURL: CODEBUFF_API_URL,
    apiKey: credential.authToken,
  });

  const convertedMessages = convertCodebuffMessages(messages);
  const convertedTools = convertCodebuffTools(tools);

  const result = streamText({
    model: codebuff.chatModel(model.id),
    system: systemPrompt,
    messages: convertedMessages,
    ...(Object.keys(convertedTools).length > 0 ? { tools: convertedTools } : {}),
    abortSignal: signal,
    // Mirrors the official SDK's getProviderOptions(): codebuff_metadata and
    // provider land at the top level of the chat-completions request body.
    providerOptions: {
      codebuff: {
        codebuff_metadata: {
          cost_mode: "free",
          run_id: randomUUID(),
          client_id: "console-agent",
        },
        provider: { allow_fallbacks: true },
      },
    },
  });

  for await (const part of result.fullStream) {
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
};
