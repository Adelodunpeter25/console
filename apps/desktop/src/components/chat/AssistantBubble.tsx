import React from "react";
import { AlertCircle } from "lucide-react";
import type { AgentMessage, ToolResult } from "@console/types";
import { MarkdownRenderer, ThinkingBlock, ToolCallBlock } from "../common";

type AssistantMessage = Extract<AgentMessage, { role: "assistant" }>;

interface AssistantBubbleProps {
  message: AssistantMessage;
  toolResults?: ToolResult[];
  hideToolCalls?: boolean;
}

/**
 * Renders an assistant message: thinking blocks, markdown text, tool calls,
 * and error states. Memoized so a streaming token re-renders only the
 * active streaming bubble, not every persisted assistant message above it.
 *
 * (Conductor rewrite lesson: wrap each message row in React.memo with a
 * stable key so a token landing in one message leaves the rest untouched.)
 */
export const AssistantBubble = React.memo(function AssistantBubble({
  message,
  toolResults = [],
  hideToolCalls = false,
}: AssistantBubbleProps) {
  const textParts = message.content.filter((c) => c.type === "text");
  const thinkingParts = message.content.filter((c) => c.type === "thinking");
  const toolCallParts = message.content.filter((c) => c.type === "toolCall");
  const isError = textParts.some((c) => c.type === "text" && c.text.startsWith("Error:"));

  if (isError) {
    return (
      <div className="rounded-xl bg-danger-muted border border-danger/30 px-4 py-3">
        <div className="flex items-start gap-2.5">
          <AlertCircle size={16} className="text-danger shrink-0 mt-0.5" />
          <div className="text-sm text-danger selectable-text">
            {textParts.map((c, i) => (
              <p key={i} className="font-mono">
                {c.type === "text" && c.text}
              </p>
            ))}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {thinkingParts.map((part, i) => (
        <ThinkingBlock key={`thinking-${i}`} text={part.type === "thinking" ? part.text : ""} />
      ))}

      {textParts.length > 0 && (
        <div className="px-1">
          <MarkdownRenderer
            content={textParts.map((c) => (c.type === "text" ? c.text : "")).join("\n\n")}
          />
        </div>
      )}

      {toolCallParts.length > 0 && !hideToolCalls && (
        <ToolCallBlock
          calls={toolCallParts
            .map((c) => (c.type === "toolCall" ? c.call : null))
            .filter((c): c is NonNullable<typeof c> => c !== null)}
          results={toolResults}
        />
      )}
    </div>
  );
});
