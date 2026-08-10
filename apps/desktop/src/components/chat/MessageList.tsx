import React from "react";
import { Virtuoso, VirtuosoHandle } from "react-virtuoso";
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

function messageKey(index: number, msg: AgentMessage): string {
  if (msg.role === "assistant" && msg.id) return msg.id;
  return `${msg.role}-${index}`;
}

export interface MessageListRef {
  scrollToBottom: () => void;
}

// Hoisted list override so it isn't recreated per render (which would remount
// the subtree on every scroll). Must forward ref per react-virtuoso contract.
// Note: no vertical margins on items — react-virtuoso measures contentRect,
// which excludes margins. Spacing lives inside each item (pb-4).
const MessageListContainer = React.forwardRef<
  HTMLDivElement,
  React.ComponentProps<"div"> & { "data-testid": string }
>(function MessageListContainer({ children, style, "data-testid": testId }, listRef) {
  return (
    <div
      ref={listRef}
      data-testid={testId}
      style={style}
      className="max-w-3xl mx-auto px-6 py-6"
    >
      {children}
    </div>
  );
});

export const MessageList = React.forwardRef<MessageListRef, MessageListProps>(
  function MessageList(
    { messages, streamingText, streamingThinking, running, runs }: MessageListProps,
    ref,
  ) {
    const isStreaming = Boolean(streamingText || streamingThinking);
    const showStreamingBubble = running || isStreaming;
    const showEmpty = messages.length === 0 && !showStreamingBubble;

    const virtuosoRef = React.useRef<VirtuosoHandle>(null);

    // Map each user message to its run activity timeline (runs are indexed by
    // user-turn order; a user message without a run gets nothing).
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

    const [showScrollButton, setShowScrollButton] = React.useState(false);
    const scrollToBottom = React.useCallback(() => {
      virtuosoRef.current?.scrollToIndex({ index: "LAST", align: "end", behavior: "smooth" });
    }, []);

    React.useImperativeHandle(
      ref,
      () => ({
        scrollToBottom,
      }),
      [scrollToBottom],
    );

    const handleRangeChanged = React.useCallback(
      (range: { startIndex: number; endIndex: number }) => {
        // Show the "scroll to bottom" affordance only when the user has
        // scrolled away from the newest message.
        const atBottom = range.endIndex >= messages.length - 1;
        setShowScrollButton(!atBottom);
      },
      [messages.length],
    );

    if (showEmpty) {
      return (
        <div className="flex-1 flex items-center justify-center">
          <p className="text-foreground-muted text-sm">Type a prompt below to start the agent.</p>
        </div>
      );
    }

    return (
      <>
        <Virtuoso<AgentMessage>
          ref={virtuosoRef}
          className="flex-1"
          data={messages}
          computeItemKey={messageKey}
          followOutput={(isAtBottom) => (isAtBottom ? "smooth" : false)}
          rangeChanged={handleRangeChanged}
          itemContent={(index, msg) => {
            const run = userMessageRunMap.get(index);
            return (
              <div className="pb-4">
                <MessageBubble message={msg} />
                {run && (
                  <RunActivity
                    activity={run}
                    running={running && index === latestUserIndex}
                  />
                )}
              </div>
            );
          }}
          components={{
            List: MessageListContainer,
            Footer: () =>
              showStreamingBubble ? (
                <StreamingBubble text={streamingText} thinking={streamingThinking} />
              ) : null,
          }}
        />

        {showScrollButton && <ScrollToBottom onClick={scrollToBottom} />}
      </>
    );
  },
);
