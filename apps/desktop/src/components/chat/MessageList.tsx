import React from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
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

    const parentRef = React.useRef<HTMLDivElement>(null);

    const [showScrollButton, setShowScrollButton] = React.useState(false);

    // Virtual row count: settled messages + one extra slot for the live
    // streaming bubble when the agent is actively generating.
    const rowCount = messages.length + (showStreamingBubble ? 1 : 0);

    const virtualizer = useVirtualizer({
      count: rowCount,
      getScrollElement: () => parentRef.current,
      estimateSize: () => 200,
      overscan: 5,
      // Chat-style scrolling: anchor to the end so the viewport follows the
      // growing streaming bubble automatically. followOnAppend scrolls to the
      // end when new messages are appended and the user was already at the
      // bottom. This replaces the old per-token autoScroll effect that forced
      // a synchronous layout reflow on every single token.
      anchorTo: "end",
      followOnAppend: true,
      scrollEndThreshold: 80,
      getItemKey: (index) => {
        if (index >= messages.length) return "streaming";
        const msg = messages[index]!;
        return messageKey(msg, index);
      },
    });

    const virtualItems = virtualizer.getVirtualItems();
    const totalSize = virtualizer.getTotalSize();

    const handleScroll = React.useCallback(() => {
      if (!parentRef.current) return;
      const { scrollTop, scrollHeight, clientHeight } = parentRef.current;
      const atBottom = scrollHeight - scrollTop - clientHeight < 60;
      setShowScrollButton(!atBottom);
    }, []);

    const scrollToBottom = React.useCallback(() => {
      virtualizer.scrollToEnd();
    }, [virtualizer]);

    // Scroll to bottom on initial load / session switch. ChatScreen loads
    // messages by first clearing to [] then loading the real array, so
    // messages.length transitions through 0 → N and this effect fires once.
    // The virtualizer's anchorTo: "end" + followOnAppend handle all streaming
    // and new-message auto-follow after that, so we do NOT depend on
    // streamingText/streamingThinking here.
    const scrollRafRef = React.useRef<number | null>(null);
    React.useLayoutEffect(() => {
      if (messages.length === 0) return;
      if (scrollRafRef.current != null) return;
      scrollRafRef.current = requestAnimationFrame(() => {
        scrollRafRef.current = null;
        virtualizer.scrollToEnd();
      });
      return () => {
        if (scrollRafRef.current != null) {
          cancelAnimationFrame(scrollRafRef.current);
          scrollRafRef.current = null;
        }
      };
    }, [virtualizer, messages.length]);

    React.useEffect(() => {
      return () => {
        if (scrollRafRef.current != null) {
          cancelAnimationFrame(scrollRafRef.current);
        }
      };
    }, []);

    React.useImperativeHandle(
      ref,
      () => ({
        scrollToBottom,
      }),
      [scrollToBottom],
    );

    return (
      <>
        <div ref={parentRef} onScroll={handleScroll} className="flex-1 overflow-y-auto pt-6 pb-6">
          {showEmpty ? (
            <div className="flex items-center justify-center h-full">
              <p className="text-foreground-muted text-sm">Type a prompt below to start the agent.</p>
            </div>
          ) : (
            <div
              className="max-w-3xl mx-auto"
              style={{ height: totalSize, position: "relative", width: "100%" }}
            >
              {virtualItems.map((virtualRow) => {
                const isStreamingRow = virtualRow.index >= messages.length;

                if (isStreamingRow) {
                  return (
                    <div
                      key="streaming"
                      ref={virtualizer.measureElement}
                      data-index={virtualRow.index}
                      className="px-6"
                      style={{
                        position: "absolute",
                        top: 0,
                        left: 0,
                        width: "100%",
                        transform: `translateY(${virtualRow.start}px)`,
                        paddingBottom: "1rem",
                      }}
                    >
                      <StreamingBubble text={streamingText} thinking={streamingThinking} />
                    </div>
                  );
                }

                const msg = messages[virtualRow.index]!;
                const key = messageKey(msg, virtualRow.index);
                return (
                  <div
                    key={key}
                    ref={virtualizer.measureElement}
                    data-index={virtualRow.index}
                    className="px-6"
                    style={{
                      position: "absolute",
                      top: 0,
                      left: 0,
                      width: "100%",
                      transform: `translateY(${virtualRow.start}px)`,
                      paddingBottom: "1rem",
                    }}
                  >
                    <MessageBubble message={msg} />
                    {userMessageRunMap.has(virtualRow.index) && (
                      <RunActivity
                        activity={userMessageRunMap.get(virtualRow.index)!}
                        running={running && virtualRow.index === latestUserIndex}
                      />
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {showScrollButton && <ScrollToBottom onClick={scrollToBottom} />}
      </>
    );
  },
);
