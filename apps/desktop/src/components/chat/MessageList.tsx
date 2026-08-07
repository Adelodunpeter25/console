import React from "react";
import type { AgentMessage } from "@console/types";
import { MessageBubble } from "./MessageBubble";
import { StreamingBubble } from "./StreamingBubble";
import { ScrollToBottom } from "./ScrollToBottom";
import { RunActivity } from "./RunActivity";
import type { RunActivityState } from "../../types/chat";
import { useCustomChatVirtualizer } from "../../hooks/useCustomChatVirtualizer";

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
  | { kind: "message"; message: AgentMessage; originalIndex: number; key: string }
  | { kind: "streaming"; key: "streaming" };

export const MessageList = React.forwardRef<MessageListRef, MessageListProps>(
  function MessageList(
    { messages, streamingText, streamingThinking, running, runs }: MessageListProps,
    ref,
  ) {
    const isStreaming = Boolean(streamingText || streamingThinking);
    const showStreamingBubble = running || isStreaming;

    // Filter out toolResult messages from virtual row items.
    // toolResult messages are transport-only items rendered as null in MessageBubble;
    // including them as virtual rows creates empty placeholder heights that cause giant gaps.
    const rowItems = React.useMemo<RowItem[]>(() => {
      const items: RowItem[] = [];
      for (let i = 0; i < messages.length; i++) {
        const msg = messages[i]!;
        if (msg.role === "toolResult") continue;
        items.push({
          kind: "message",
          message: msg,
          originalIndex: i,
          key: messageKey(msg, i),
        });
      }
      if (showStreamingBubble) {
        items.push({ kind: "streaming", key: "streaming" });
      }
      return items;
    }, [messages, showStreamingBubble]);

    const showEmpty = rowItems.length === 0;

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

    const {
      parentRef,
      handleScroll: handleVirtualScroll,
      startIndex,
      endIndex,
      topSpacerHeight,
      bottomSpacerHeight,
      measureRef,
      scrollToEnd,
    } = useCustomChatVirtualizer({
      items: rowItems,
      estimateSize: () => 150,
      overscan: 5,
    });

    const [showScrollButton, setShowScrollButton] = React.useState(false);

    const handleScroll = React.useCallback(() => {
      handleVirtualScroll();
      if (!parentRef.current) return;
      const { scrollTop, scrollHeight, clientHeight } = parentRef.current;
      const atBottom = scrollHeight - scrollTop - clientHeight < 60;
      setShowScrollButton(!atBottom);
    }, [handleVirtualScroll]);

    // Auto-scroll ONLY when a new prompt is submitted by the user.
    const prevMessagesLengthRef = React.useRef(messages.length);
    React.useEffect(() => {
      const prevLength = prevMessagesLengthRef.current;
      prevMessagesLengthRef.current = messages.length;

      if (messages.length > prevLength) {
        const lastMsg = messages[messages.length - 1];
        if (lastMsg?.role === "user") {
          requestAnimationFrame(() => {
            scrollToEnd();
          });
        }
      }
    }, [messages, scrollToEnd]);

    React.useImperativeHandle(
      ref,
      () => ({
        scrollToBottom: scrollToEnd,
      }),
      [scrollToEnd],
    );

    const visibleRowItems = rowItems.slice(startIndex, endIndex + 1);

    return (
      <>
        <div ref={parentRef} onScroll={handleScroll} className="flex-1 overflow-y-auto pt-6 pb-6">
          {showEmpty ? (
            <div className="flex items-center justify-center h-full">
              <p className="text-foreground-muted text-sm">Type a prompt below to start the agent.</p>
            </div>
          ) : (
            <div className="max-w-3xl mx-auto px-6">
              <div style={{ height: topSpacerHeight }} />
              <div className="space-y-4">
                {visibleRowItems.map((item, sliceIndex) => {
                  const virtualIndex = startIndex + sliceIndex;

                  if (item.kind === "streaming") {
                    return (
                      <div
                        key="streaming"
                        ref={(node) => measureRef(virtualIndex, node)}
                      >
                        <StreamingBubble text={streamingText} thinking={streamingThinking} />
                      </div>
                    );
                  }

                  const msg = item.message;
                  return (
                    <div
                      key={item.key}
                      ref={(node) => measureRef(virtualIndex, node)}
                    >
                      <MessageBubble message={msg} />
                      {userMessageRunMap.has(item.originalIndex) && (
                        <RunActivity
                          activity={userMessageRunMap.get(item.originalIndex)!}
                          running={running && item.originalIndex === latestUserIndex}
                        />
                      )}
                    </div>
                  );
                })}
              </div>
              <div style={{ height: bottomSpacerHeight }} />
            </div>
          )}
        </div>

        {showScrollButton && <ScrollToBottom onClick={scrollToEnd} />}
      </>
    );
  },
);
