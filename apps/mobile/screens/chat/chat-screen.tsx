import React, { useState, useCallback, useEffect, useRef, useMemo } from "react";
import { View, Keyboard, BackHandler, Pressable } from "react-native";
import { SquareTerminal, Folder } from "lucide-react-native";
import type { LegendListRef } from "@legendapp/list/react-native";
import { useChatStream, useAbort, useSessionTodos } from "@/hooks";
import { sessionsView$ } from "@/stores/useSessionStore";
import { ScreenHeader } from "@/components/layout/screen-header";
import {
  ChatMessageList,
  ChatEmptyState,
  ChatScrollBottomButton,
  Composer,
  InteractionPanel,
  TodoBanner,
  TodoBottomSheet,
} from "@/components/chat";
import { ChatScreenSkeleton } from "@/components/common";
import { app$, setActiveTab, setSelectedProjectId, setSelectedSessionId } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";
import { project$ } from "@/stores/useProjectStore";

export function ChatScreen() {
  const stream = useChatStream();
  const { abort } = useAbort();
  const selectedSessionId = useValue(app$.selectedSessionId);
  const projects = useValue(project$.projects);
  const sessionCwd = useValue(() =>
    selectedSessionId ? sessionsView$[selectedSessionId].sessionCwd.get() ?? null : null,
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
  const listRef = useRef<LegendListRef>(null);
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

  const {
    todoItems,
    totalCount,
    completedCount,
    hasActiveTodos,
    nextPendingTodo,
    bottomSheetRef,
    openSheet,
  } = useSessionTodos(selectedSessionId);

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
        <>
          {hasActiveTodos ? (
            <TodoBanner
              completedCount={completedCount}
              totalCount={totalCount}
              nextTask={nextPendingTodo?.content}
              onPress={openSheet}
            />
          ) : null}
          <InteractionPanel sessionId={selectedSessionId} />
        </>
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
          topBanner={
            hasActiveTodos ? (
              <TodoBanner
                completedCount={completedCount}
                totalCount={totalCount}
                nextTask={nextPendingTodo?.content}
                onPress={openSheet}
              />
            ) : null
          }
        />
      )}

      {/* Expandable Task Checklist Bottom Sheet */}
      <TodoBottomSheet
        ref={bottomSheetRef}
        todoItems={todoItems}
        completedCount={completedCount}
        totalCount={totalCount}
      />
    </View>
  );
}

export default ChatScreen;