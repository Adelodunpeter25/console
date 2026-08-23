import React, { useState, useCallback, useEffect, useRef, useMemo } from "react";
import { View, Keyboard, BackHandler, Pressable } from "react-native";
import { SquareTerminal } from "lucide-react-native";
import type { FlashListRef } from "@shopify/flash-list";
import type { AgentMessage } from "@console/types";
import { useChatStream, useAbort } from "../../hooks";
import {
  useAppStore,
  useSessionStore,
  useProjectStore,
} from "../../stores";
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
  const setSelectedProjectId = useAppStore((state) => state.setSelectedProjectId);

  // Header action: jump to the terminal screen scoped to this session's project,
  // so the shell opens in the session's cwd instead of an arbitrary fallback.
  const openTerminal = useCallback(() => {
    const session = selectedSessionId
      ? useSessionStore.getState().getSession(selectedSessionId)
      : undefined;
    const cwd = session?.sessionCwd;
    const match = cwd
      ? useProjectStore
          .getState()
          .projects.find((p) => p.path === cwd || p.path.endsWith(cwd))
      : undefined;
    if (match) setSelectedProjectId(match.id);
    setActiveTab("terminal");
  }, [selectedSessionId, setSelectedProjectId, setActiveTab]);

  const headerTerminalButton = (
    <Pressable
      className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
      style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
      onPress={openTerminal}
    >
      <SquareTerminal size={18} color="#ffffff" />
    </Pressable>
  );
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
      <ScreenHeader title={stream.chatTitle} onBack={handleBackToHome} rightAction={headerTerminalButton} />

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