/**
 * Converts AgentMessage[] to Gemini Content[] wire format.
 *
 * Mapping:
 *   UserMessage          → role: "user",  parts: [GeminiTextPart]
 *   AssistantMessage     → role: "model", parts: GeminiTextPart | GeminiFunctionCallPart
 *   ToolResultMessage    → role: "user",  parts: [GeminiFunctionResponsePart]
 */
import type { AgentMessage } from "../../../agent/src/types/index.js";
import type {
  GeminiContent,
  GeminiFunctionCallPart,
  GeminiFunctionCallRef,
  GeminiFunctionResponseBody,
  GeminiFunctionResponsePart,
  GeminiFunctionResponseRef,
  GeminiOutgoingPart,
  GeminiTextPart,
} from "../types/index.js";

function makeTextPart(text: string): GeminiTextPart {
  return { text };
}

function makeFunctionCallPart(name: string, args: Record<string, unknown>, id: string): GeminiFunctionCallPart {
  const ref: GeminiFunctionCallRef = { name, args, id };
  return { functionCall: ref };
}

function makeFunctionResponsePart(name: string, id: string, content: unknown): GeminiFunctionResponsePart {
  const body: GeminiFunctionResponseBody = { content };
  const ref: GeminiFunctionResponseRef = { name, id, response: body };
  return { functionResponse: ref };
}

/**
 * Normalize tool call IDs to be safe for wire format.
 * Based on oh-my-pi reference: replace non-alphanumeric characters with underscores.
 */
function normalizeToolCallId(id: string): string {
  return id.replace(/[^a-zA-Z0-9_-]/g, "_").slice(0, 64);
}

export function convertMessages(messages: AgentMessage[]): GeminiContent[] {
  const contents: GeminiContent[] = [];

  for (const msg of messages) {
    if (msg.role === "user") {
      // Skip empty user messages
      if (!msg.content || msg.content.trim() === "") {
        continue;
      }
      const content: GeminiContent = {
        role: "user",
        parts: [makeTextPart(msg.content)],
      };
      contents.push(content);
      continue;
    }

    if (msg.role === "assistant") {
      const parts: GeminiOutgoingPart[] = [];

      for (const part of msg.content) {
        if (part.type === "text" && part.text) {
          parts.push(makeTextPart(part.text));
        } else if (part.type === "toolCall") {
          const args = (part.call.arguments ?? {}) as Record<string, unknown>;
          const normalizedId = normalizeToolCallId(part.call.id);
          parts.push(makeFunctionCallPart(part.call.name, args, normalizedId));
        }
      }

      // Only add assistant message if it has content
      if (parts.length > 0) {
        contents.push({ role: "model", parts });
      }
      continue;
    }

    if (msg.role === "toolResult") {
      const parts: GeminiOutgoingPart[] = msg.results.map((r) =>
        makeFunctionResponsePart(r.toolCallId, normalizeToolCallId(r.toolCallId), r.content),
      );
      // Only add tool result message if it has results
      if (parts.length > 0) {
        contents.push({ role: "user", parts });
      }
    }
  }

  return contents;
}
