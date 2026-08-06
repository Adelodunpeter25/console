/**
 * Converts AgentMessage[] to OpenAI chat.completions wire format.
 *
 * Mapping:
 *   UserMessage       → role: "user", content (text, plus image_url parts if attached)
 *   AssistantMessage  → role: "assistant", content, tool_calls, reasoning_content
 *   ToolResultMessage → role: "tool", tool_call_id, content (JSON string)
 */
import type { AgentMessage } from "../../../agent/src/types/index.js";

export interface OpenAIInputMessage {
  role: "system" | "user" | "assistant" | "tool";
  content: string | Array<Record<string, unknown>> | null;
  name?: string;
  tool_call_id?: string;
  tool_calls?: Array<{
    id: string;
    type: "function";
    function: { name: string; arguments: string };
  }>;
  reasoning_content?: string;
}

export function convertOpencodeMessages(
  messages: AgentMessage[],
  systemPrompt: string,
): OpenAIInputMessage[] {
  const out: OpenAIInputMessage[] = [];

  if (systemPrompt.trim()) {
    out.push({ role: "system", content: systemPrompt });
  }

  for (const msg of messages) {
    if (msg.role === "user") {
      if (!msg.content || msg.content.trim() === "") continue;

      if (msg.attachments && msg.attachments.length > 0) {
        const parts: Array<Record<string, unknown>> = [
          { type: "text", text: msg.content },
          ...msg.attachments.map((att) => ({
            type: "image_url" as const,
            image_url: { url: `data:${att.mimeType};base64,${att.data}` },
          })),
        ];
        out.push({ role: "user", content: parts });
      } else {
        out.push({ role: "user", content: msg.content });
      }
      continue;
    }

    if (msg.role === "assistant") {
      const textParts: string[] = [];
      const toolCalls: OpenAIInputMessage["tool_calls"] = [];
      const reasoningParts: string[] = [];

      for (const part of msg.content) {
        if (part.type === "thinking") {
          reasoningParts.push(part.text);
        } else if (part.type === "text" && part.text) {
          textParts.push(part.text);
        } else if (part.type === "toolCall") {
          toolCalls.push({
            id: part.call.id,
            type: "function",
            function: {
              name: part.call.name,
              arguments:
                typeof part.call.arguments === "string"
                  ? part.call.arguments
                  : JSON.stringify(part.call.arguments ?? {}),
            },
          });
        }
      }

      const content = textParts.length > 0 ? textParts.join("\n") : null;

      const m: OpenAIInputMessage = {
        role: "assistant",
        content,
        ...(reasoningParts.length > 0
          ? { reasoning_content: reasoningParts.join("\n") }
          : {}),
        ...(toolCalls.length > 0 ? { tool_calls: toolCalls } : {}),
      };
      out.push(m);
      continue;
    }

    if (msg.role === "toolResult") {
      for (const r of msg.results) {
        out.push({
          role: "tool",
          tool_call_id: r.toolCallId,
          content:
            typeof r.content === "string" ? r.content : JSON.stringify(r.content ?? null),
        });
      }
    }
  }

  return out;
}
