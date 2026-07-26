import React from "react";
import type { AgentMessage, AssistantMessageContent, ToolCall } from "@console/types";
import { MessageBubble } from "./MessageBubble";
import { StreamingBubble } from "./StreamingBubble";
import { ScrollToBottom } from "./ScrollToBottom";
import { ToolCallBlock } from "../common";

interface MessageListProps {
  messages: AgentMessage[];
  streamingText: string;
  streamingThinking: string;
  running: boolean;
}

/**
 * Stable, collision-free key for any AgentMessage.
 * Assistant messages carry an optional `id`; user/toolResult rows fall back
 * to a role+index composite. Keeping this in one place ensures the list (and
 * any future virtualizer) always receives stable, predictable keys.
 */
function messageKey(msg: AgentMessage, index: number): string {
  if (msg.role === "assistant" && msg.id) return msg.id;
  return `${msg.role}-${index}`;
}

/**
 * Extract tool calls from an assistant message (used to show pending calls
 * while the agent is executing tools and to pair results with their calls).
 */
function assistantToolCalls(msg: AgentMessage | undefined): ToolCall[] {
  if (!msg || msg.role !== "assistant") return [];
  return msg.content
    .filter((c): c is Extract<AssistantMessageContent, { type: "toolCall" }> => c.type === "toolCall")
    .map((c) => c.call);
}

/**
 * Scrollable, virtualization-ready message list.
 *
 * Design (Conductor rewrite lessons):
 *  - Settled message rows are rendered via memoized `MessageBubble`s with
 *    stable keys, so a token landing in the streaming bubble does NOT
 *    re-render the hundreds of messages above it.
 *  - The single `StreamingBubble` is rendered separately and intentionally
 *    not memoized — it is the only row that updates per token.
 *  - The list is data-driven (`messages` + streaming props) so it can be
 *    dropped into `react-virtuoso` later with stable item keys + `followOutput`
 *    without changing call sites. See the Conductor findings artifact.
 */
export function MessageList({
  messages,
  streamingText,
  streamingThinking,
  running,
}: MessageListProps) {
  const scrollRef = React.useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = React.useState(true);

  // Auto-scroll to bottom when new content arrives (unless the user scrolled up).
  React.useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, streamingText, streamingThinking, autoScroll]);

  const handleScroll = React.useCallback(() => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    const atBottom = scrollHeight - scrollTop - clientHeight < 60;
    setAutoScroll(atBottom);
  }, []);

  const scrollToBottom = React.useCallback(() => {
    setAutoScroll(true);
    if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, []);

  const isStreaming = Boolean(streamingText || streamingThinking);
  const showEmpty = messages.length === 0 && !isStreaming;

  // Pending tool calls from the latest assistant message, shown while the
  // agent is executing tools and no streaming text is being emitted.
  const lastAssistant = React.useMemo(
    () => [...messages].reverse().find((m) => m.role === "assistant"),
    [messages],
  );
  const pendingToolCalls = React.useMemo(
    () => (running && !streamingText ? assistantToolCalls(lastAssistant) : []),
    [running, streamingText, lastAssistant],
  );

  const showScrollButton = !autoScroll && (isStreaming || running);

  return (
    <>
      <div ref={scrollRef} onScroll={handleScroll} className="flex-1 overflow-y-auto">
        {showEmpty ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-foreground-muted text-sm">
              Type a prompt below to start the agent.
            </p>
          </div>
        ) : (
          <div className="max-w-3xl mx-auto px-6 py-6 space-y-4">
            {messages.map((msg, i) => (
              <MessageBubble
                key={messageKey(msg, i)}
                message={msg}
                prevMessage={messages[i - 1]}
              />
            ))}
            {isStreaming && (
              <StreamingBubble text={streamingText} thinking={streamingThinking} />
            )}
            {pendingToolCalls.length > 0 && <ToolCallBlock calls={pendingToolCalls} />}
          </div>
        )}
      </div>

      {showScrollButton && <ScrollToBottom onClick={scrollToBottom} />}
    </>
  );
}
