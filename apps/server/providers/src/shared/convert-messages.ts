/**
 * Converts AgentMessage[] to Gemini Content[] wire format.
 *
 * Mapping:
 *   UserMessage          → role: "user",  parts: [GeminiTextPart]
 *   AssistantMessage     → role: "model", parts: GeminiTextPart | GeminiFunctionCallPart
 *   ToolResultMessage    → role: "user",  parts: [GeminiFunctionResponsePart]
 */
import type { AgentMessage } from "@console/types";
import type {
  GeminiContent,
  GeminiFunctionCallPart,
  GeminiFunctionCallRef,
  GeminiFunctionResponseBody,
  GeminiFunctionResponsePart,
  GeminiFunctionResponseRef,
  GeminiInlineDataPart,
  GeminiOutgoingPart,
  GeminiTextPart,
} from "@/providers/src/types/index.js";

function makeTextPart(text: string, thoughtSignature?: string): GeminiTextPart {
  return {
    text,
    ...(thoughtSignature ? { thoughtSignature } : {}),
  };
}

function makeInlineDataPart(data: string, mimeType: string): GeminiInlineDataPart {
  return { inlineData: { mimeType, data } };
}

function makeFunctionCallPart(
  name: string,
  args: Record<string, unknown>,
  id: string,
  thoughtSignature?: string,
): GeminiFunctionCallPart {
  const ref: GeminiFunctionCallRef = { name, args, id };
  return {
    functionCall: ref,
    ...(thoughtSignature ? { thoughtSignature } : {}),
  };
}

/** Used only for legacy histories created before signatures were persisted. */
export const LEGACY_THOUGHT_SIGNATURE = "skip_thought_signature_validator";

function makeFunctionResponsePart(
  name: string,
  id: string,
  content: unknown,
): GeminiFunctionResponsePart {
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
  // 1. Build toolCallId -> toolName lookup map from assistant tool calls in history
  const toolNameByCallId = new Map<string, string>();
  for (const msg of messages) {
    if (msg.role === "assistant") {
      for (const part of msg.content) {
        if (part.type === "toolCall") {
          toolNameByCallId.set(part.call.id, part.call.name);
          toolNameByCallId.set(normalizeToolCallId(part.call.id), part.call.name);
        }
      }
    }
  }

  const rawTurns: GeminiContent[] = [];

  for (const msg of messages) {
    if (msg.role === "user") {
      // Skip empty user messages
      if (!msg.content || msg.content.trim() === "") {
        continue;
      }
      const parts: GeminiOutgoingPart[] = [makeTextPart(msg.content)];
      // Inline image attachments become Gemini inlineData parts.
      for (const att of msg.attachments ?? []) {
        parts.push(makeInlineDataPart(att.data, att.mimeType));
      }
      rawTurns.push({ role: "user", parts });
      continue;
    }

    if (msg.role === "assistant") {
      const parts: GeminiOutgoingPart[] = [];

      for (const part of msg.content) {
        if (part.type === "text" && (part.text || part.thoughtSignature)) {
          parts.push(makeTextPart(part.text, part.thoughtSignature));
        } else if (part.type === "toolCall") {
          const args = (part.call.arguments ?? {}) as Record<string, unknown>;
          const normalizedId = normalizeToolCallId(part.call.id);
          parts.push(
            makeFunctionCallPart(
              part.call.name,
              args,
              normalizedId,
              part.call.thoughtSignature ?? LEGACY_THOUGHT_SIGNATURE,
            ),
          );
        }
      }

      // Only add assistant message if it has content
      if (parts.length > 0) {
        rawTurns.push({ role: "model", parts });
      }
      continue;
    }

    if (msg.role === "toolResult") {
      const parts: GeminiOutgoingPart[] = msg.results.map((r) => {
        const toolName =
          r.toolName ||
          toolNameByCallId.get(r.toolCallId) ||
          toolNameByCallId.get(normalizeToolCallId(r.toolCallId)) ||
          r.toolCallId;
        return makeFunctionResponsePart(toolName, normalizeToolCallId(r.toolCallId), r.content);
      });
      // Only add tool result message if it has results
      if (parts.length > 0) {
        rawTurns.push({ role: "user", parts });
      }
    }
  }

  // 2. Merge adjacent turns of the same role (Gemini strict role alternation invariant)
  const mergedTurns: GeminiContent[] = [];
  for (const turn of rawTurns) {
    if (turn.parts.length === 0) continue;
    const last = mergedTurns[mergedTurns.length - 1];
    if (last && last.role === turn.role) {
      last.parts.push(...turn.parts);
    } else {
      mergedTurns.push({ role: turn.role, parts: [...turn.parts] });
    }
  }

  // 3. Enforce conversation begins and ends with user when user turns exist
  // (prevents "This model does not support assistant message prefill")
  const hasUserTurn = mergedTurns.some((t) => t.role === "user");
  if (hasUserTurn) {
    while (mergedTurns.length > 0 && mergedTurns[0]!.role !== "user") {
      mergedTurns.shift();
    }
    while (mergedTurns.length > 0 && mergedTurns[mergedTurns.length - 1]!.role !== "user") {
      mergedTurns.pop();
    }
  }

  return mergedTurns;
}
