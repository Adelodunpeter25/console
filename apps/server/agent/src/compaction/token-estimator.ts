import type { AgentMessage } from "@/agent/src/types/index.js";

/**
 * Estimate token count for a list of messages.
 * Uses ~4 chars per token for text and ~1,000 tokens per inline image attachment.
 */
export function estimateMessageTokens(messages: AgentMessage[]): number {
  let totalChars = 0;
  let imageTokens = 0;

  for (const msg of messages) {
    if (msg.role === "user") {
      totalChars += msg.content ? msg.content.length : 0;
      if (msg.attachments && msg.attachments.length > 0) {
        imageTokens += msg.attachments.length * 1000;
      }
    } else if (msg.role === "assistant") {
      for (const part of msg.content) {
        if (part.type === "text" || part.type === "thinking") {
          totalChars += part.text ? part.text.length : 0;
        } else if (part.type === "toolCall") {
          totalChars += part.call.name.length;
          if (part.call.arguments) {
            try {
              totalChars += JSON.stringify(part.call.arguments).length;
            } catch {
              totalChars += 50;
            }
          }
        }
      }
    } else if (msg.role === "toolResult") {
      for (const res of msg.results) {
        if (typeof res.content === "string") {
          totalChars += res.content.length;
        } else if (res.content != null) {
          try {
            totalChars += JSON.stringify(res.content).length;
          } catch {
            totalChars += 100;
          }
        }
      }
    }
  }

  return Math.ceil(totalChars / 4) + imageTokens;
}
