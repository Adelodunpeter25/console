import React, { useState, useCallback, useEffect, useRef } from "react";
import { View, Keyboard, BackHandler } from "react-native";
import type { FlashListRef } from "@shopify/flash-list";
import type { AgentMessage } from "@console/types";
import { useChatStream, useAbort } from "../../hooks";
import { useAppStore } from "../../stores";
import { ScreenHeader } from "../../components/layout/screen-header";
import {
  ChatMessageList,
  ChatEmptyState,
  ChatScrollBottomButton,
  Composer,
  InteractionPanel,
} from "../../components/chat";
import { ChatScreenSkeleton } from "../../components/common";

export function ChatScreen() {
  const stream = useChatStream();
  const { abort } = useAbort();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const listRef = useRef<FlashListRef<AgentMessage>>(null);
  const [showScrollBottom, setShowScrollBottom] = useState(false);

  const handleBackToHome = useCallback(() => {
    setSelectedSessionId(null);
    setActiveTab("home");
  }, [setSelectedSessionId, setActiveTab]);

  useEffect(() => {
    const onBackPress = () => {
      handleBackToHome();
      return true;
    };
    const sub = BackHandler.addEventListener("hardwareBackPress", onBackPress);
    return () => sub.remove();
  }, [handleBackToHome]);

  const handleScrollToEnd = useCallback(() => {
    setShowScrollBottom(false);
    listRef.current?.scrollToEnd({ animated: true });
  }, []);

  const handleStop = useCallback(() => {
    stream.stop();
    abort();
  }, [stream, abort]);

  const hasPendingInteraction = useMemo(
    () =>
      (stream.pendingPermissions?.length ?? 0) > 0 ||
      (stream.pendingQuestions?.length ?? 0) > 0 ||
      Boolean(stream.pendingPermission) ||
      Boolean(stream.pendingQuestion),
    [
      stream.pendingPermissions?.length,
      stream.pendingQuestions?.length,
      stream.pendingPermission,
      stream.pendingQuestion,
    ],
  );

  const isStreaming = useMemo(
    () =>
      stream.running &&
      (Boolean(stream.streamingText) ||
        Boolean(stream.streamingThinking) ||
        stream.activeToolCalls.length > 0),
    [stream.running, stream.streamingText, stream.streamingThinking, stream.activeToolCalls.length],
  );

  const hasMessages = useMemo(() => stream.messages.length > 0, [stream.messages.length]);

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader title={stream.chatTitle} onBack={handleBackToHome} />

      {stream.isLoadingMessages && !hasMessages ? (
        <ChatScreenSkeleton />
      ) : !hasMessages && !isStreaming ? (
        <ChatEmptyState />
      ) : (
        <ChatMessageList
          ref={listRef}
          stream={stream}
          onScrollBottomVisibilityChange={setShowScrollBottom}
        />
      )}

      {/* Floating scroll to bottom button */}
      <ChatScrollBottomButton
        visible={showScrollBottom}
        onPress={handleScrollToEnd}
        hasInteraction={hasPendingInteraction}
      />

      {/* Footer interaction panel or composer */}
      {hasPendingInteraction ? (
        <InteractionPanel sessionId={selectedSessionId} />
      ) : (
        <Composer
          value={stream.inputVal}
          onChangeText={stream.setInputVal}
          onSend={() => {
            Keyboard.dismiss();
            handleScrollToEnd();
            stream.sendMessage();
          }}
          onStop={handleStop}
          running={stream.running}
          projectLocked={stream.messages.length > 0}
        />
      )}
    </View>
  );
}

export default ChatScreen;