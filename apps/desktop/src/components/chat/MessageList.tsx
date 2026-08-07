import React from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";
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

function messageKey(msg: AgentMessage, index: number): string {
  if (msg.role === "assistant" && msg.id) return msg.id;
  return `${msg.role}-${index}`;
}

export interface MessageListRef {
  scrollToBottom: () => void;
}

type RowItem =
  | { kind: "message"; message: AgentMessage; index: number; key: string }
  | { kind: "streaming"; key: "streaming" };

export const MessageList = React.forwardRef<MessageListRef, MessageListProps>(
  function MessageList(
    { messages, streamingText, streamingThinking, running, runs }: MessageListProps,
    ref,
  ) {
    const virtuosoRef = React.useRef<VirtuosoHandle>(null);
    const [showScrollButton, setShowScrollButton] = React.useState(false);

    const isStreaming = Boolean(streamingText || streamingThinking);
    const showStreamingBubble = running || isStreaming;
    const showEmpty = messages.length === 0 && !showStreamingBubble;

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

    const rowItems = React.useMemo<RowItem[]>(() => {
      const items: RowItem[] = messages.map((msg, index) => ({
        kind: "message",
        message: msg,
        index,
        key: messageKey(msg, index),
      }));
      if (showStreamingBubble) {
        items.push({ kind: "streaming", key: "streaming" });
      }
      return items;
    }, [messages, showStreamingBubble]);

    const scrollToBottom = React.useCallback(() => {
      if (virtuosoRef.current) {
        virtuosoRef.current.scrollToIndex({
          index: rowItems.length - 1,
          behavior: "smooth",
        });
      }
    }, [rowItems.length]);

    React.useImperativeHandle(
      ref,
      () => ({
        scrollToBottom,
      }),
      [scrollToBottom],
    );

    if (showEmpty) {
      return (
        <div className="flex-1 flex items-center justify-center h-full">
          <p className="text-foreground-muted text-sm">Type a prompt below to start the agent.</p>
        </div>
      );
    }

    return (
      <div className="flex-1 relative h-full w-full overflow-hidden">
        <Virtuoso
          ref={virtuosoRef}
          data={rowItems}
          computeItemKey={(_index, item) => item.key}
          initialTopMostItemIndex={rowItems.length > 0 ? rowItems.length - 1 : 0}
          followOutput={(isAtBottom) => (isAtBottom ? "auto" : false)}
          alignToBottom
          className="h-full w-full"
          atBottomStateChange={(atBottom) => {
            setShowScrollButton(!atBottom);
          }}
          itemContent={(_index, item) => {
            if (item.kind === "streaming") {
              return (
                <div className="max-w-3xl mx-auto px-6 py-2">
                  <StreamingBubble text={streamingText} thinking={streamingThinking} />
                </div>
              );
            }

            const msg = item.message;
            return (
              <div className="max-w-3xl mx-auto px-6 py-2">
                <MessageBubble message={msg} />
                {userMessageRunMap.has(item.index) && (
                  <RunActivity
                    activity={userMessageRunMap.get(item.index)!}
                    running={running && item.index === latestUserIndex}
                  />
                )}
              </div>
            );
          }}
        />

        {showScrollButton && <ScrollToBottom onClick={scrollToBottom} />}
      </div>
    );
  },
);
