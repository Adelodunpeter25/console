import React from "react";
import type { AgentMessage } from "@console/types";
import { MessageBubble } from "./MessageBubble";
import { StreamingBubble } from "./StreamingBubble";
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

export function MessageList({
  messages,
  streamingText,
  streamingThinking,
  running,
  runs,
}: MessageListProps) {
  const isStreaming = Boolean(streamingText || streamingThinking);
  const showStreamingBubble = running || isStreaming;
  const showEmpty = messages.length === 0 && !showStreamingBubble;

  // Map each user message to its corresponding run.
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
    estimateSize: 100,
    overscan: 5,
  });

  return (
    <div ref={parentRef} className="flex-1 overflow-y-auto">
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
                <div
                  key={messageKey(msg, i)}
                  ref={virtualizer.measureElement}
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
                  <MessageBubble message={msg} />
                  {userMessageRunMap.has(i) && (
                    <RunActivity
                      activity={userMessageRunMap.get(i)!}
                      running={running && i === latestUserIndex}
                    />
                  )}
                </div>
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
  );
}
