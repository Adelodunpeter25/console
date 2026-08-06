/**
 * Converts AgentMessage[] to AI SDK UIMessage[] for the opencode provider.
 *
 * Mapping:
 *   UserMessage       → role: "user", content (string or parts with images)
 *   AssistantMessage  → role: "assistant", content parts (text/reasoning/tool-call)
 *   ToolResultMessage → role: "tool", content: [{ type: "tool-result", ... }]
 */
import type { AgentMessage } from "@console/types";
import type { UIMessage } from "ai";

export function convertOpencodeMessages(messages: AgentMessage[]): UIMessage[] {
  const out: UIMessage[] = [];

  for (const msg of messages) {
    if (msg.role === "user") {
      if (!msg.content || msg.content.trim() === "") continue;

      if (msg.attachments && msg.attachments.length > 0) {
        out.push({
          id: `user-${out.length}`,
          role: "user",
          content: [
            { type: "text", text: msg.content },
            ...msg.attachments.map((att) => ({
              type: "image" as const,
              image: `data:${att.mimeType};base64,${att.data}`,
            })),
          ],
        });
      } else {
        out.push({ id: `user-${out.length}`, role: "user", content: msg.content });
      }
      continue;
    }

    if (msg.role === "assistant") {
      const content: UIMessage["content"] = [];

      for (const part of msg.content) {
        if (part.type === "thinking" && part.text) {
          content.push({ type: "reasoning", text: part.text });
        } else if (part.type === "text" && part.text) {
          content.push({ type: "text", text: part.text });
        } else if (part.type === "toolCall") {
          content.push({
            type: "tool-call",
            toolCallId: part.call.id,
            toolName: part.call.name,
            args:
              typeof part.call.arguments === "string"
                ? JSON.parse(part.call.arguments)
                : (part.call.arguments ?? {}),
          });
        }
      }

      out.push({ id: `assistant-${out.length}`, role: "assistant", content });
      continue;
    }

    if (msg.role === "toolResult") {
      for (const r of msg.results) {
        out.push({
          id: `tool-${out.length}`,
          role: "tool",
          content: [
            {
              type: "tool-result",
              toolCallId: r.toolCallId,
              result: r.content,
              isError: r.isError,
            },
          ],
        });
      }
    }
  }

  return out;
}
