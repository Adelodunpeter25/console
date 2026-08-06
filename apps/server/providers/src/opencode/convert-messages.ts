/**
 * Converts AgentMessage[] to AI SDK CoreMessage[] for the opencode provider.
 *
 * Mapping:
 *   UserMessage       → role: "user", content (string or parts with images)
 *   AssistantMessage  → role: "assistant", content parts (text/reasoning/tool-call)
 *   ToolResultMessage → role: "tool", content: [{ type: "tool-result", ... }]
 */
import type { AgentMessage } from "@console/types";
import type { ModelMessage } from "ai";

export function convertOpencodeMessages(messages: AgentMessage[]): ModelMessage[] {
  const out: ModelMessage[] = [];

  for (const msg of messages) {
    if (msg.role === "user") {
      if (!msg.content || msg.content.trim() === "") continue;

      if (msg.attachments && msg.attachments.length > 0) {
        out.push({
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
        out.push({ role: "user", content: msg.content });
      }
      continue;
    }

    if (msg.role === "assistant") {
      const content: any[] = [];
      let hasReasoning = false;

      for (const part of msg.content) {
        if (part.type === "thinking" && part.text) {
          content.push({ type: "reasoning", text: part.text });
          hasReasoning = true;
        } else if (part.type === "text" && part.text) {
          content.push({ type: "text", text: part.text });
        } else if (part.type === "toolCall") {
          content.push({
            type: "tool-call",
            toolCallId: part.call.id,
            toolName: part.call.name,
            input:
              typeof part.call.arguments === "string"
                ? JSON.parse(part.call.arguments)
                : (part.call.arguments ?? {}),
          });
        }
      }

      if (!hasReasoning) {
        content.push({ type: "reasoning", text: " " });
      }

      out.push({ role: "assistant", content });
      continue;
    }

    if (msg.role === "toolResult") {
      for (const r of msg.results) {
        const toolName = r.toolName || findToolName(r.toolCallId, messages);
        out.push({
          role: "tool",
          content: [
            {
              type: "tool-result",
              toolCallId: r.toolCallId,
              toolName,
              output: getToolResultOutput(r),
            },
          ],
        } as any);
      }
    }
  }

  return out;
}

function getToolResultOutput(r: { content: any; isError?: boolean }): any {
  if (r.isError) {
    return {
      type: "error-text",
      value: typeof r.content === "string" ? r.content : JSON.stringify(r.content),
    };
  }

  if (typeof r.content === "string") {
    return {
      type: "text",
      value: r.content,
    };
  }

  return {
    type: "json",
    value: r.content,
  };
}

function findToolName(toolCallId: string, messages: AgentMessage[]): string {
  for (const msg of messages) {
    if (msg.role === "assistant") {
      for (const part of msg.content) {
        if (part.type === "toolCall" && part.call.id === toolCallId) {
          return part.call.name;
        }
      }
    }
  }
  return "";
}
