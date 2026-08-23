import React, { useState, useCallback, useEffect, useRef, useMemo } from "react";
import { View, Keyboard, BackHandler, Pressable } from "react-native";
import { SquareTerminal, Folder } from "lucide-react-native";
import type { FlashListRef } from "@shopify/flash-list";
import type { AgentMessage } from "@console/types";
import { useChatStream, useAbort } from "@/hooks";
import { useAppStore, useSessionStore, useProjectStore } from "@/stores";
import { ScreenHeader } from "@/components/layout/screen-header";
import {
  ChatMessageList,
  ChatEmptyState,
  ChatScrollBottomButton,
  Composer,
  InteractionPanel,
} from "@/components/chat";
import { ChatScreenSkeleton } from "@/components/common";

export function ChatScreen() {
  const stream = useChatStream();
  const { abort } = useAbort();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const setSelectedProjectId = useAppStore((state) => state.setSelectedProjectId);
  const projects = useProjectStore((state) => state.projects);
  const sessionCwd = useSessionStore((state) =>
    selectedSessionId ? state.sessions[selectedSessionId]?.sessionCwd ?? null : null,
  );

  const findProjectForCwd = useCallback(
    (cwd: string | null) => {
      if (!cwd) return undefined;
      return projects.find((p) => p.path === cwd || cwd.startsWith(p.path + "/") || p.path.endsWith(cwd));
    },
    [projects],
  );

  const openTerminal = useCallback(() => {
    const match = findProjectForCwd(sessionCwd);
    if (match) setSelectedProjectId(match.id);
    setActiveTab("terminal");
  }, [sessionCwd, findProjectForCwd, setSelectedProjectId, setActiveTab]);

  const openFiles = useCallback(() => {
    const match = findProjectForCwd(sessionCwd);
    if (match) setSelectedProjectId(match.id);
    setActiveTab("files");
  }, [sessionCwd, findProjectForCwd, setSelectedProjectId, setActiveTab]);

  const headerRightActions = useMemo(
    () => (
      <View className="flex-row items-center gap-2">
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Open file explorer"
          hitSlop={8}
          className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
          style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
          onPress={openFiles}
        >
          <Folder size={18} color="#ffffff" />
        </Pressable>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Open terminal"
          hitSlop={8}
          className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
          style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
          onPress={openTerminal}
        >
          <SquareTerminal size={18} color="#ffffff" />
        </Pressable>
      </View>
    ),
    [openFiles, openTerminal],
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
      <ScreenHeader title={stream.chatTitle} onBack={handleBackToHome} rightAction={headerRightActions} />

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