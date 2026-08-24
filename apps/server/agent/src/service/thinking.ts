/**
 * Extended Thinking & Reasoning Parser.
 * Separates model reasoning/thinking tokens (<thinking>...</thinking>) from visible response text.
 * Inspired by oh-my-pi/packages/coding-agent/src/thinking.ts.
 */
import type { ThinkingPart, TextPart } from "@/agent/src/types/index.js";

export interface ThinkingParseResult {
  textParts: TextPart[];
  thinkingParts: ThinkingPart[];
}

/**
 * Parse text content to extract <thinking>...</thinking> tag blocks.
 */
export function extractThinkingFromText(rawText: string): ThinkingParseResult {
  const thinkingParts: ThinkingPart[] = [];
  const textParts: TextPart[] = [];

  const thinkingRegex = /<thinking>([\s\S]*?)<\/thinking>/gi;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = thinkingRegex.exec(rawText)) !== null) {
    // Text before the thinking block
    if (match.index > lastIndex) {
      const preceding = rawText.slice(lastIndex, match.index);
      if (preceding) textParts.push({ type: "text", text: preceding });
    }

    const thinkingContent = match[1] ?? "";
    if (thinkingContent) {
      thinkingParts.push({ type: "thinking", text: thinkingContent });
    }

    lastIndex = thinkingRegex.lastIndex;
  }

  // Text after the last thinking block
  if (lastIndex < rawText.length) {
    const trailing = rawText.slice(lastIndex);
    if (trailing) textParts.push({ type: "text", text: trailing });
  }

  return { textParts, thinkingParts };
}
