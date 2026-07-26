import React from "react";
import type { AgentMessage, AssistantMessageContent, ToolCall } from "@console/types";
import { ToolCallBlock } from "../common";
import { UserBubble } from "./UserBubble";
import { AssistantBubble } from "./AssistantBubble";

interface MessageBubbleProps {
  message: AgentMessage;
  prevMessage?: AgentMessage;
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

  return <AssistantBubble message={message} />;
});
