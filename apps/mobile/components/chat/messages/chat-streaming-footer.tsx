import React, { memo } from "react";
import { View } from "react-native";
import type { RunActivityState } from "@/types/chat";
import { MessageBubble } from "./message-bubbles";
import { RunActivity } from "../tools/run-activity";

interface ChatStreamingFooterProps {
  latestUserIndex: number;
  effectiveRuns: RunActivityState[];
  running: boolean;
  isStreaming: boolean;
  streamingThinking?: string;
  streamingText?: string;
}

export const ChatStreamingFooter = memo(function ChatStreamingFooter({
  latestUserIndex,
  effectiveRuns,
  running,
  isStreaming,
  streamingThinking,
  streamingText,
}: ChatStreamingFooterProps) {
  const showUnattachedRun = latestUserIndex === -1 && effectiveRuns.length > 0;
  const showStreamingBubble = isStreaming && (Boolean(streamingText) || Boolean(streamingThinking));

  if (!showUnattachedRun && !showStreamingBubble) return null;

  return (
    <View>
      {/* If running without a user message row yet, show latest run activity */}
      {showUnattachedRun ? (
        <RunActivity
          activity={effectiveRuns[effectiveRuns.length - 1]!}
          running={running}
        />
      ) : null}

      {/* Streaming response bubble */}
      {showStreamingBubble ? (
        <MessageBubble
          isStreaming
          item={{
            role: "assistant",
            content: [
              ...(streamingThinking ? [{ type: "thinking" as const, text: streamingThinking }] : []),
              ...(streamingText ? [{ type: "text" as const, text: streamingText }] : []),
            ],
          }}
        />
      ) : null}
    </View>
  );
});
