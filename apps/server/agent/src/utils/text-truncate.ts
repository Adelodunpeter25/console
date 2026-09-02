import type { AgentMessage, ToolResult } from "@/agent/src/types/index.js";

/** Default character budget per tool result content (~2,000 tokens). */
export const DEFAULT_TOOL_RESULT_MAX_CHARS = 8_000;

/**
 * Truncate a large text string by keeping the head and tail, inserting
 * a clear notice in the middle detailing how many characters were elided.
 */
export function truncateHeadTail(
  text: string,
  maxChars = DEFAULT_TOOL_RESULT_MAX_CHARS,
  headRatio = 0.5,
): string {
  if (maxChars <= 0 || text.length <= maxChars) {
    return text;
  }

  const headChars = Math.max(0, Math.floor(maxChars * headRatio));
  const tailChars = Math.max(0, maxChars - headChars);
  const elidedChars = text.length - headChars - tailChars;

  if (elidedChars <= 0) {
    return text;
  }

  const head = text.slice(0, headChars);
  const tail = tailChars > 0 ? text.slice(-tailChars) : "";
  const marker = `\n\n[... Tool output truncated: ${elidedChars.toLocaleString()} characters elided ...]\n\n`;

  return `${head}${marker}${tail}`;
}

/**
 * Truncate content in a ToolResult if it exceeds maxChars.
 */
export function truncateToolResultContent(
  content: unknown,
  maxChars = DEFAULT_TOOL_RESULT_MAX_CHARS,
): unknown {
  if (maxChars <= 0 || content == null) {
    return content;
  }

  if (typeof content === "string") {
    return truncateHeadTail(content, maxChars);
  }

  if (Array.isArray(content)) {
    // Array of content parts, e.g. [{ type: "text", text: "..." }]
    let totalLen = 0;
    for (const item of content) {
      if (item && typeof item === "object" && "text" in item && typeof item.text === "string") {
        totalLen += item.text.length;
      }
    }
    if (totalLen > maxChars) {
      // Truncate text blocks proportionally or convert to truncated string
      return content.map((item) => {
        if (item && typeof item === "object" && "text" in item && typeof item.text === "string") {
          return {
            ...item,
            text: truncateHeadTail(item.text, maxChars),
          };
        }
        return item;
      });
    }
    return content;
  }

  // If it's an arbitrary object, check JSON representation
  try {
    const jsonStr = JSON.stringify(content);
    if (jsonStr.length > maxChars) {
      return truncateHeadTail(jsonStr, maxChars);
    }
  } catch {
    // Fall back to untruncated content if serialization fails
  }

  return content;
}

/**
 * In-place / functional truncation of tool results in an AgentMessage.
 */
export function truncateMessageToolResults(
  msg: AgentMessage,
  maxChars = DEFAULT_TOOL_RESULT_MAX_CHARS,
): AgentMessage {
  if (maxChars <= 0 || msg.role !== "toolResult") {
    return msg;
  }

  const truncatedResults: ToolResult[] = msg.results.map((res) => {
    const truncatedContent = truncateToolResultContent(res.content, maxChars);
    if (truncatedContent === res.content) {
      return res;
    }
    return {
      ...res,
      content: truncatedContent,
    };
  });

  return {
    ...msg,
    results: truncatedResults,
  };
}
