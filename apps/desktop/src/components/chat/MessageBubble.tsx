import React from "react";
import type { AgentMessage } from "@console/types";
import { UserBubble } from "./UserBubble";
import { AssistantBubble } from "./AssistantBubble";

interface MessageBubbleProps {
  message: AgentMessage;
}

/**
 * Dispatcher that routes a message to the correct bubble component based on
 * its role. Memoized so the parent MessageList can re-render on every token
 * without re-rendering already-settled messages.
 *
 * Tool calls are NEVER rendered here — they are rendered exclusively in
 * per-run `RunActivity` blocks. `toolResult` messages are persistence
 * transport only and render as null.
 *
 * (Conductor rewrite lesson: React.memo + stable key per row means only the
 * streaming bubble re-renders, not the hundreds of messages above it.)
 */
export const MessageBubble = React.memo(function MessageBubble({
  message,
}: MessageBubbleProps) {
  if (message.role === "user") {
    return <UserBubble content={message.content} attachments={message.attachments} />;
  }

  if (message.role === "toolResult") {
    // Tool results are persistence transport only — they're rendered in
    // RunActivity blocks, not inline.
    return null;
  }

  return <AssistantBubble message={message} />;
});
