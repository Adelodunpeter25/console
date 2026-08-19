import React, { useCallback, useMemo, useRef } from "react";
import { View, Text, Keyboard, type NativeScrollEvent, type NativeSyntheticEvent } from "react-native";
import { FlashList, type FlashListRef } from "@shopify/flash-list";
import { MessageSquareText } from "lucide-react-native";
import type { AgentMessage } from "@console/types";
import { useChatStream, useAbort, useChatDecisions } from "../../hooks";
import { useAppStore } from "../../stores";
import { ScreenHeader } from "../../components/layout/screen-header";
import { MessageBubble } from "../../components/chat/message-bubbles";
import { RunActivity } from "../../components/chat/run-activity";
import { Composer } from "../../components/chat/composer";
import { InteractionPanel } from "../../components/chat/interaction-panel";
import { reconstructRuns } from "../../utils/reconstruct-runs";
import { theme } from "../../styles/theme";

function isVisibleMessage(msg: AgentMessage): boolean {
  if (msg.role === "toolResult") return false;
  if (msg.role === "assistant" && msg.content.some((c) => c.type === "toolCall")) {
    return false;
  }
  return true;
}

export function ChatScreen() {
  const stream = useChatStream();
  const { abort, isAborting } = useAbort();
  const decisions = useChatDecisions();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const chatTitle = stream.chatTitle;
  const listRef = useRef<FlashListRef<AgentMessage>>(null);

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
      if (!isStreaming) {
        followRef.current = atEnd;
      }
    },
    [isStreaming],
  );

  const handleScrollToEnd = useCallback(() => {
    isAtEndRef.current = true;
    followRef.current = true;
    listRef.current?.scrollToEnd({ animated: true });
  }, []);

  const handleStop = useCallback(() => {
    stream.stop();
    abort();
  }, [stream, abort]);

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
    <View className="flex-1 bg-screen">
      <ScreenHeader title={chatTitle} onBack={() => setActiveTab("home")} />

      {displayMessages.length === 0 && !isStreaming ? (
        <View className="flex-1 items-center justify-center px-8">
          <View
            className="w-14 h-14 rounded-2xl items-center justify-center mb-4"
            style={{ backgroundColor: "rgba(255,255,255,0.06)" }}
          >
            <MessageSquareText size={24} color={theme.colors.text.secondary} />
          </View>
          <Text className="text-foreground text-base font-semibold mb-1.5 text-center">
            Start a conversation
          </Text>
          <Text className="text-foreground-secondary text-sm text-center leading-5">
            Ask the agent to write code, review a change, or run commands on your project.
          </Text>
        </View>
      ) : (
        <FlashList
          ref={listRef}
          className="flex-1"
          data={displayMessages}
          keyExtractor={(_, i) => i.toString()}
          contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 4, paddingBottom: 16 }}
          keyboardShouldPersistTaps="handled"
          keyboardDismissMode="interactive"
          onScroll={handleScroll}
          scrollEventThrottle={32}
          onContentSizeChange={() => {
            if (followRef.current || isStreaming) {
              handleScrollToEnd();
            }
          }}
          renderItem={renderItem}
          ListFooterComponent={
            <View>
              {/* If running without a user message row yet, show latest run activity */}
              {latestUserIndex === -1 && effectiveRuns.length > 0 ? (
                <RunActivity
                  activity={effectiveRuns[effectiveRuns.length - 1]!}
                  running={stream.running}
                />
              ) : null}
              {isStreaming && (Boolean(stream.streamingText) || Boolean(stream.streamingThinking)) ? (
                <MessageBubble
                  isStreaming
                  item={{
                    role: "assistant",
                    content: [
                      ...(stream.streamingThinking
                        ? [{ type: "thinking" as const, text: stream.streamingThinking }]
                        : []),
                      ...(stream.streamingText
                        ? [{ type: "text" as const, text: stream.streamingText }]
                        : []),
                    ],
                  }}
                />
              ) : null}
            </View>
          }
        />
      )}

      <InteractionPanel sessionId={selectedSessionId} />

      <Composer
        value={stream.inputVal}
        onChangeText={stream.setInputVal}
        onSend={() => {
          Keyboard.dismiss();
          handleScrollToEnd();
          stream.sendMessage();
        }}
        onStop={isAborting ? undefined : handleStop}
        running={stream.running}
      />
    </View>
  );
}

export default ChatScreen;