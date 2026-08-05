import React from "react";
import type { AgentMessage } from "@console/types";
import { MessageBubble } from "./MessageBubble";
import { StreamingBubble } from "./StreamingBubble";
import { ScrollToBottom } from "./ScrollToBottom";
import { RunActivity } from "./RunActivity";
import type { RunActivityState } from "../../types/chat";

interface MessageListProps {
  messages: AgentMessage[];
  streamingText: string;
  streamingThinking: string;
  running: boolean;
  runs: RunActivityState[];
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
 * Scrollable, virtualization-ready message list.
 *
 * Design (Conductor rewrite lessons):
 *  - Settled message rows are rendered via memoized `MessageBubble`s with
 *    stable keys, so a token landing in the streaming bubble does NOT
 *    re-render the hundreds of messages above it.
 *  - The single `StreamingBubble` is rendered separately and intentionally
 *    not memoized — it is the only row that updates per token.
 *  - Tool calls are rendered exclusively inside per-run `RunActivity`
 *    blocks, never inline in assistant bubbles. This means starting a new
 *    prompt never invalidates the completion state of earlier runs.
 */
export function MessageList({
  messages,
  streamingText,
  streamingThinking,
  running,
  runs,
}: MessageListProps) {
  const scrollRef = React.useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = React.useState(true);

  // Auto-scroll to bottom when new content arrives (unless the user scrolled up).
  React.useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, streamingText, streamingThinking, autoScroll, runs]);

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
  const showStreamingBubble = running || isStreaming;
  const showEmpty = messages.length === 0 && !showStreamingBubble;

  // Map each user message to its corresponding run. runs[k] corresponds to
  // the k-th user message (0-indexed).
  const userMessageRunMap = React.useMemo(() => {
    const map = new Map<number, RunActivityState>();
    let userCount = 0;
    for (let i = 0; i < messages.length; i++) {
      if (messages[i]!.role === "user") {
        const run = runs[userCount];
        if (run) map.set(i, run);
        userCount++;
      }
    }
    return map;
  }, [messages, runs]);

  const latestUserIndex = React.useMemo(
    () => messages.findLastIndex((message) => message.role === "user"),
    [messages],
  );

  const showScrollButton = !autoScroll;

  return (
    <>
      <div ref={scrollRef} onScroll={handleScroll} className="flex-1 overflow-y-auto">
        {showEmpty ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-foreground-muted text-sm">Type a prompt below to start the agent.</p>
          </div>
        ) : (
          <div className="max-w-3xl mx-auto px-6 py-6 space-y-4">
            {messages.map((msg, i) => (
              <React.Fragment key={messageKey(msg, i)}>
                <MessageBubble message={msg} />
                {userMessageRunMap.has(i) && (
                  <RunActivity
                    activity={userMessageRunMap.get(i)!}
                    running={running && i === latestUserIndex}
                  />
                )}
              </React.Fragment>
            ))}
            {showStreamingBubble && (
              <StreamingBubble text={streamingText} thinking={streamingThinking} />
            )}
          </div>
        )}
      </div>

      {showScrollButton && <ScrollToBottom onClick={scrollToBottom} />}
    </>
  );
}
