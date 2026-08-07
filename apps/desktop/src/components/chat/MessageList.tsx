import React from "react";
import type { AgentMessage } from "@console/types";
import { MessageBubble } from "./MessageBubble";
import { StreamingBubble } from "./StreamingBubble";
import { ScrollToBottom } from "./ScrollToBottom";
import { RunActivity } from "./RunActivity";
import type { RunActivityState } from "../../types/chat";
import { useVirtualList } from "../../hooks/useVirtualList";

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

/** Component wrapper per virtualized row with ResizeObserver for dynamic measurement */
function VirtualizedRow({
  virtualRow,
  virtualizer,
  children,
}: {
  virtualRow: { index: number; start: number };
  virtualizer: any;
  children: React.ReactNode;
}) {
  const rowRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (!rowRef.current) return;
    // Measure element immediately on mount
    virtualizer.measureElement(rowRef.current);

    // Attach ResizeObserver to remeasuring on code block highlight, thought dropdown toggle, image load
    const observer = new ResizeObserver(() => {
      if (rowRef.current) {
        virtualizer.measureElement(rowRef.current);
      }
    });

    observer.observe(rowRef.current);
    return () => observer.disconnect();
  }, [virtualizer]);

  return (
    <div
      ref={rowRef}
      data-index={virtualRow.index}
      style={{
        position: "absolute",
        top: 0,
        left: 0,
        width: "100%",
        transform: `translateY(${virtualRow.start}px)`,
      }}
      className="pb-4"
    >
      {children}
    </div>
  );
}

export const MessageList = React.forwardRef<MessageListRef, MessageListProps>(
  function MessageList(
    { messages, streamingText, streamingThinking, running, runs }: MessageListProps,
    ref,
  ) {
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

    const { parentRef, virtualizer, virtualItems, totalSize } = useVirtualList({
      items: messages,
      estimateSize: 120,
      overscan: 8,
    });

    const [showScrollButton, setShowScrollButton] = React.useState(false);

    const handleScroll = React.useCallback(() => {
      if (!parentRef.current) return;
      const { scrollTop, scrollHeight, clientHeight } = parentRef.current;
      const atBottom = scrollHeight - scrollTop - clientHeight < 60;
      setShowScrollButton(!atBottom);
    }, [parentRef]);

    const scrollToBottom = React.useCallback(() => {
      if (parentRef.current) {
        parentRef.current.scrollTop = parentRef.current.scrollHeight;
      }
    }, [parentRef]);

    React.useImperativeHandle(
      ref,
      () => ({
        scrollToBottom,
      }),
      [scrollToBottom],
    );

    return (
      <>
        <div ref={parentRef} onScroll={handleScroll} className="flex-1 overflow-y-auto">
          {showEmpty ? (
            <div className="flex items-center justify-center h-full">
              <p className="text-foreground-muted text-sm">Type a prompt below to start the agent.</p>
            </div>
          ) : (
            <div className="max-w-3xl mx-auto px-6 py-6">
              <div
                style={{
                  height: `${totalSize}px`,
                  width: "100%",
                  position: "relative",
                }}
              >
                {virtualItems.map((virtualRow) => {
                  const msg = messages[virtualRow.index];
                  if (!msg) return null;
                  const i = virtualRow.index;

                  return (
                    <VirtualizedRow
                      key={messageKey(msg, i)}
                      virtualRow={virtualRow}
                      virtualizer={virtualizer}
                    >
                      <MessageBubble message={msg} />
                      {userMessageRunMap.has(i) && (
                        <RunActivity
                          activity={userMessageRunMap.get(i)!}
                          running={running && i === latestUserIndex}
                        />
                      )}
                    </VirtualizedRow>
                  );
                })}
              </div>
              {showStreamingBubble && (
                <div className="mt-4">
                  <StreamingBubble text={streamingText} thinking={streamingThinking} />
                </div>
              )}
            </div>
          )}
        </div>

        {showScrollButton && <ScrollToBottom onClick={scrollToBottom} />}
      </>
    );
  },
);
