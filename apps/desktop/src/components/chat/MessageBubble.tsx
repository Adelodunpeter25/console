import React from "react";
import type { AgentMessage, AssistantMessageContent, ToolCall, ToolResult } from "@console/types";
import { ToolCallBlock } from "../common";
import { UserBubble } from "./UserBubble";
import { AssistantBubble } from "./AssistantBubble";

interface MessageBubbleProps {
  message: AgentMessage;
  prevMessage?: AgentMessage;
  nextMessage?: AgentMessage;
  /** Real-time tool results from `toolExecutionResult` events, used to
      update tool call status before `toolExecutionEnd` finalises the batch. */
  liveToolResults?: ToolResult[];
}

/**
 * Dispatcher that routes a message to the correct bubble component based on
 * its role. Memoized so the parent MessageList can re-render on every token
 * without re-rendering already-settled messages.
 *
 * (Conductor rewrite lesson: React.memo + stable key per row means only the
 * streaming bubble re-renders, not the hundreds of messages above it.)
 */
export const MessageBubble = React.memo(function MessageBubble({
  message,
  prevMessage,
  nextMessage,
  liveToolResults = [],
}: MessageBubbleProps) {
  if (message.role === "user") {
    return <UserBubble content={message.content} />;
  }

  if (message.role === "toolResult") {
    // Tool results are rendered inline after the assistant message that
    // contained the tool calls — pull the matching calls from the previous
    // assistant message.
    const prevCalls: ToolCall[] =
      prevMessage?.role === "assistant"
        ? prevMessage.content
            .filter(
              (c): c is Extract<AssistantMessageContent, { type: "toolCall" }> =>
                c.type === "toolCall",
            )
            .map((c) => c.call)
        : [];
    return <ToolCallBlock calls={prevCalls} results={message.results} />;
  }

  // Assistant message — merge results from the following toolResult message
  // with any live results that arrived before the batch finalised. This
  // ensures tool call spinners flip to checkmarks in real-time as each
  // tool completes, not only after all tools finish.
  const toolResults: ToolResult[] =
    nextMessage?.role === "toolResult" ? nextMessage.results : [];

  // Merge: prefer finalised results, add any live results not yet in the list.
  const mergedResults: ToolResult[] = [...toolResults];
  for (const live of liveToolResults) {
    if (!mergedResults.some((r) => r.toolCallId === live.toolCallId)) {
      mergedResults.push(live);
    }
  }

  return <AssistantBubble message={message} toolResults={mergedResults} />;
});
