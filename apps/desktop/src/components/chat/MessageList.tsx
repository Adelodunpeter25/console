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
  },
);
