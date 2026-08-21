import React, { forwardRef, useCallback, useMemo, useRef } from "react";
import { View, Keyboard, type NativeScrollEvent, type NativeSyntheticEvent } from "react-native";
import { FlashList, type FlashListRef } from "@shopify/flash-list";
import type { AgentMessage } from "@console/types";
import { MessageBubble } from "./message-bubbles";
import { RunActivity } from "./run-activity";
import { ChatPaginationHeader } from "./chat-pagination-header";
import { ChatStreamingFooter } from "./chat-streaming-footer";
import { reconstructRuns } from "../../utils/reconstruct-runs";
import type { useChatStream } from "../../hooks/useChatStream";

function isVisibleMessage(msg: AgentMessage): boolean {
  if (msg.role === "toolResult") return false;
  if (msg.role === "assistant" && msg.content.some((c) => c.type === "toolCall")) {
    return false;
  }
  return true;
}

interface ChatMessageListProps {
  stream: ReturnType<typeof useChatStream>;
  onScrollBottomVisibilityChange: (visible: boolean) => void;
}

export const ChatMessageList = forwardRef<FlashListRef<AgentMessage>, ChatMessageListProps>(
  function ChatMessageList({ stream, onScrollBottomVisibilityChange }, ref) {
    const isStreaming =
      stream.running &&
      (Boolean(stream.streamingText) ||
        Boolean(stream.streamingThinking) ||
        stream.activeToolCalls.length > 0);

    const displayMessages = useMemo(() => {
      return stream.messages.filter(isVisibleMessage);
    }, [stream.messages]);

    const effectiveRuns = useMemo(() => {
      if (stream.runs && stream.runs.length > 0) return stream.runs;
      return reconstructRuns(stream.messages);
    }, [stream.runs, stream.messages]);

    const userMessageRunMap = useMemo(() => {
      const map = new Map<number, (typeof effectiveRuns)[number]>();
      let userCount = 0;
      for (let i = 0; i < displayMessages.length; i++) {
        if (displayMessages[i]!.role === "user") {
          const run = effectiveRuns[userCount];
          if (run) map.set(i, run);
          userCount++;
        }
      }
      return map;
    }, [displayMessages, effectiveRuns]);

    const latestUserIndex = useMemo(
      () => displayMessages.findLastIndex((message) => message.role === "user"),
      [displayMessages],
    );

    const isAtEndRef = useRef(true);
    const followRef = useRef(true);

    const handleScroll = useCallback(
      (event: NativeSyntheticEvent<NativeScrollEvent>) => {
        const { layoutMeasurement, contentOffset, contentSize } = event.nativeEvent;
        const distanceFromEnd =
          contentSize.height - (layoutMeasurement.height + contentOffset.y);
        const atEnd = distanceFromEnd < 96;
        isAtEndRef.current = atEnd;
        onScrollBottomVisibilityChange(!atEnd && displayMessages.length > 2);
        if (!isStreaming) {
          followRef.current = atEnd;
        }
        // Top reached: auto-paginate earlier messages
        if (contentOffset.y < 60 && stream.hasEarlierMessages && !stream.isFetchingEarlierMessages) {
          stream.fetchEarlierMessages();
        }
      },
      [
        isStreaming,
        displayMessages.length,
        stream.hasEarlierMessages,
        stream.isFetchingEarlierMessages,
        stream.fetchEarlierMessages,
        onScrollBottomVisibilityChange,
      ],
    );

    const handleScrollToEnd = useCallback(() => {
      isAtEndRef.current = true;
      followRef.current = true;
      onScrollBottomVisibilityChange(false);
      (ref as React.RefObject<FlashListRef<AgentMessage>>)?.current?.scrollToEnd({ animated: true });
    }, [ref, onScrollBottomVisibilityChange]);

    const renderItem = useCallback(
      ({ item, index }: { item: AgentMessage; index: number }) => {
        const run = userMessageRunMap.get(index);
        return (
          <View className="mb-1">
            <MessageBubble item={item} />
            {run ? (
              <RunActivity
                activity={run}
                running={stream.running && index === latestUserIndex}
              />
            ) : null}
          </View>
        );
      },
      [userMessageRunMap, stream.running, latestUserIndex],
    );

    return (
      <FlashList
        ref={ref}
        className="flex-1"
        data={displayMessages}
        estimatedItemSize={110}
        keyExtractor={(item, i) => (item as any).id ?? `${(item as any).createdAt ?? i}-${i}`}
        contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 4, paddingBottom: 16 }}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        onScrollBeginDrag={() => Keyboard.dismiss()}
        onScroll={handleScroll}
        scrollEventThrottle={32}
        onContentSizeChange={() => {
          if (followRef.current || isStreaming) {
            handleScrollToEnd();
          }
        }}
        renderItem={renderItem}
        ListHeaderComponent={
          <ChatPaginationHeader
            hasEarlierMessages={stream.hasEarlierMessages}
            isFetchingEarlierMessages={stream.isFetchingEarlierMessages}
            onFetchEarlierMessages={stream.fetchEarlierMessages}
          />
        }
        ListFooterComponent={
          <ChatStreamingFooter
            latestUserIndex={latestUserIndex}
            effectiveRuns={effectiveRuns}
            running={stream.running}
            isStreaming={isStreaming}
            streamingThinking={stream.streamingThinking}
            streamingText={stream.streamingText}
          />
        }
      />
    );
  },
);
