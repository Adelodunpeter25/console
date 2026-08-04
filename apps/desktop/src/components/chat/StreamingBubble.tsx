import React from "react";
import { MarkdownRenderer, ThinkingBlock } from "../common";

interface StreamingBubbleProps {
  text: string;
  thinking: string;
}

/**
 * Live streaming bubble shown while the agent is generating a response.
 * Not memoized — this is the one component that SHOULD re-render on every
 * token. All other message bubbles above it are memoized and stay untouched.
 *
 * (Conductor rewrite lesson: only re-render what actually changed.)
 */
export function StreamingBubble({ text, thinking }: StreamingBubbleProps) {
  return (
    <div className="space-y-2">
      {thinking && <ThinkingBlock text={thinking} isStreaming={true} />}

      {text && (
        <div className="px-1">
          <MarkdownRenderer content={text} streaming />
        </div>
      )}

      {!text && !thinking && (
        <div className="flex items-center gap-2 text-foreground-muted">
          <div className="w-2 h-2 rounded-full bg-foreground-muted animate-pulse" />
          <span className="text-xs">Agent is thinking...</span>
        </div>
      )}
    </div>
  );
}
