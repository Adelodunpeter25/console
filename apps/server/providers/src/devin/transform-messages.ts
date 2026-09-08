/**
 * Console AgentMessage → Devin ChatMessagePrompt conversion.
 *
 * Adapted from oh-my-pi's provider-side transformMessages. The Console
 * AgentMessage shape uses discriminated unions on `role` + `content`
 * (parts with `type`); Devin's wire shape uses `source` enum + plain
 * fields, so the conversion is mostly structural.
 */
import * as crypto from "node:crypto";
import { zodToJsonSchema } from "zod-to-json-schema";
import {
  ChatMessagePromptSchema,
  ChatMessageSource,
  ChatToolCallSchema,
  ImageDataSchema,
  type ChatMessagePrompt,
  type ImageData,
  type ChatToolCall,
} from "./proto/devin-proto.js";
import { create } from "./proto/protobuf.js";
import type { AgentMessage, AgentTool, AssistantMessageContent } from "@console/types";

/** Flatten one user turn into a Cascade USER prompt with inline images. */
function buildUserPrompt(msg: Extract<AgentMessage, { role: "user" }>, messageId: string): ChatMessagePrompt {
  const images: ImageData[] = (msg.attachments ?? []).map((att) =>
    create(ImageDataSchema, { base64Data: att.data, mimeType: att.mimeType }),
  );
  return create(ChatMessagePromptSchema, {
    messageId,
    source: ChatMessageSource.USER,
    prompt: msg.content,
    images,
  });
}

/** Cascade SYSTEM prompt for assistant turns; preserves text + thinking + tool calls. */
function buildAssistantPrompt(
  msg: Extract<AgentMessage, { role: "assistant" }>,
  messageId: string,
): ChatMessagePrompt {
  let promptText = "";
  let thinkingText = "";
  let signature = "";
  const toolCalls: ChatToolCall[] = [];

  for (const part of msg.content as AssistantMessageContent[]) {
    if (part.type === "text") {
      promptText += part.text;
      // Text parts may carry a thought signature for Gemini.
      const sig = part.thoughtSignature;
      if (sig && !signature) signature = sig;
    } else if (part.type === "thinking") {
      thinkingText += part.text;
    } else if (part.type === "toolCall") {
      toolCalls.push(
        create(ChatToolCallSchema, {
          id: part.call.id,
          name: part.call.name,
          argumentsJson: JSON.stringify(part.call.arguments ?? {}),
        }),
      );
    }
  }

  return create(ChatMessagePromptSchema, {
    messageId,
    source: ChatMessageSource.SYSTEM,
    prompt: promptText,
    thinking: thinkingText,
    signature,
    signatureType: "",
    toolCalls,
  });
}

/** Cascade TOOL prompt — one per tool result entry in the message. */
function buildToolPrompt(
  msg: Extract<AgentMessage, { role: "toolResult" }>,
  messageId: string,
  toolCallId: string,
): ChatMessagePrompt {
  // Tool-result messages carry an array of results; each result becomes its
  // own Cascade tool prompt. The caller passes the toolCallId for the slot
  // being filled; we serialize the matching result's text output.
  let resultText = "";
  for (const result of msg.results) {
    if (result.toolCallId !== toolCallId) continue;
    if (result.content !== undefined) resultText = JSON.stringify(result.content);
    break;
  }
  return create(ChatMessagePromptSchema, {
    messageId,
    source: ChatMessageSource.TOOL,
    toolCallId,
    toolResultIsError: false,
    prompt: resultText,
    images: [] as ImageData[],
  });
}

/** Walk the agent's history into the per-turn Cascade prompts. */
export function buildChatMessagePrompts(
  messages: AgentMessage[],
  cascadeId: string,
): ChatMessagePrompt[] {
  const out: ChatMessagePrompt[] = [];
  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i]!;
    if (msg.role === "user") {
      out.push(buildUserPrompt(msg, crypto.randomUUID()));
      continue;
    }
    if (msg.role === "assistant") {
      const prompt = buildAssistantPrompt(msg, `bot-${crypto.randomUUID()}`);
      // Skip empty assistant turns.
      if (
        prompt.prompt ||
        prompt.thinking ||
        prompt.signature ||
        (prompt.toolCalls && prompt.toolCalls.length > 0)
      ) {
        out.push(prompt);
      }
      continue;
    }
    if (msg.role === "toolResult") {
      // Emit one TOOL prompt per result in the bundle.
      for (const result of msg.results) {
        out.push(buildToolPrompt(msg, crypto.randomUUID(), result.toolCallId));
      }
      continue;
    }
    // system / developer messages are flattened into the request's `prompt`
    // field upstream; skip them here.
    void cascadeId;
  }
  return out;
}

/** Devin tool definitions are JSON-schema strings — pass through. */
export function buildDevinTools(tools: AgentTool[]): {
  name: string;
  description: string;
  jsonSchemaString: string;
  strict: boolean;
}[] {
  return tools.map((tool) => ({
    name: tool.name,
    description: tool.description,
    jsonSchemaString: JSON.stringify(
      zodToJsonSchema(tool.inputSchema, { target: "openApi3", $refStrategy: "none" }),
    ),
    strict: false,
  }));
}
