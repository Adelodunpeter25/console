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

function isVisibleMessage(msg: AgentMessage): boolean {
  if (msg.role === "toolResult") return false;
  if (msg.role === "assistant" && msg.content.some((c) => c.type === "toolCall")) {
    return false;
  }
  return true;
}

function messageKey(index: number, msg: AgentMessage): string {
  if (msg.role === "assistant" && msg.id) return msg.id;
  return `${msg.role}-${index}`;
}

export interface MessageListRef {
  scrollToBottom: () => void;
}

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

    // Filter hidden messages out before passing to Virtuoso so no item renders 0px
    const displayMessages = React.useMemo(() => {
      return messages.filter(isVisibleMessage);
    }, [messages]);

    const showEmpty = displayMessages.length === 0 && !showStreamingBubble;
    const virtuosoRef = React.useRef<VirtuosoHandle>(null);

    // Map each visible user message to its run activity timeline
    const userMessageRunMap = React.useMemo(() => {
      const map = new Map<number, RunActivityState>();
      let userCount = 0;
      for (let i = 0; i < displayMessages.length; i++) {
        if (displayMessages[i]!.role === "user") {
          const run = runs[userCount];
          if (run) map.set(i, run);
          userCount++;
        }
      }
      return map;
    }, [displayMessages, runs]);

    const latestUserIndex = React.useMemo(
      () => displayMessages.findLastIndex((message) => message.role === "user"),
      [displayMessages],
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
        const atBottom = range.endIndex >= displayMessages.length - 1;
        setShowScrollButton(!atBottom);
      },
      [displayMessages.length],
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
          data={displayMessages}
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
                <div className="max-w-3xl mx-auto px-6 py-2">
                  <StreamingBubble text={streamingText} thinking={streamingThinking} />
                </div>
              ) : null,
          }}
        />

        {showScrollButton && <ScrollToBottom onClick={scrollToBottom} />}
      </>
    );
  },
);
