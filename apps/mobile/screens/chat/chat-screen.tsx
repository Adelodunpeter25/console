import React, { useCallback, useRef } from "react";
import {
  Platform,
  View,
  Text,
  type NativeScrollEvent,
  type NativeSyntheticEvent,
} from "react-native";
import { FlashList, type FlashListRef } from "@shopify/flash-list";
import { MessageSquareText } from "lucide-react-native";
import { KeyboardAvoidingView } from "react-native-keyboard-controller";
import { useSession } from "@console/api";
import { useChatStream, useAbort, useChatDecisions } from "../../hooks";
import { useAppStore } from "../../stores";
import { ScreenHeader } from "../../components/layout/screen-header";
import { MessageBubble } from "../../components/chat/message-bubbles";
import { LiveToolResults } from "../../components/chat/live-tool-results";
import { Composer } from "../../components/chat/composer";
import { ApprovalPanel } from "../../components/chat/approval-panel";
import { theme } from "../../styles/theme";

export function ChatScreen() {
  const stream = useChatStream();
  const { abort, isAborting } = useAbort();
  const decisions = useChatDecisions();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const { data: sessionDetail } = useSession(selectedSessionId ?? "");
  const chatTitle = sessionDetail?.header.title ?? "Console";
  const listRef = useRef<FlashListRef<(typeof stream.messages)[number]>>(null);

  const isStreaming =
    stream.running &&
    (!!stream.streamingText || !!stream.streamingThinking || stream.activeToolCalls.length > 0);

  // The user is "at the bottom" (the list's end) by default. While streaming we
  // follow the growing message regardless; when idle we only auto-scroll if the
  // user never scrolled away, so reading older history isn't yanked.
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
    ({ item, index }: { item: (typeof stream.messages)[number]; index: number }) => (
      <MessageBubble key={index} item={item} />
    ),
    [],
  );

  return (
    // Android already resizes the window (adjustResize). Extra KAV padding
    // double-counts and leaves the composer floating mid-screen.
    <KeyboardAvoidingView
      style={{ flex: 1, backgroundColor: "#0a0a0b" }}
      behavior="padding"
      enabled={Platform.OS === "ios"}
    >
      <ScreenHeader title={chatTitle} onBack={() => setActiveTab("home")} />

      {stream.messages.length === 0 && !isStreaming ? (
        <View style={{ flex: 1, alignItems: "center", justifyContent: "center", paddingHorizontal: 32 }}>
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
          style={{ flex: 1 }}
          data={stream.messages}
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
              <LiveToolResults results={stream.liveToolResults} />
              {isStreaming ? (
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

      <ApprovalPanel
        pendingPermission={stream.pendingPermission}
        pendingQuestion={stream.pendingQuestion}
        onApprove={async (allow) => {
          const req = stream.pendingPermission?.request;
          if (req) await decisions.approve(req.requestId, allow);
        }}
        onAnswer={async (answer) => {
          const req = stream.pendingQuestion?.request;
          if (req) await decisions.answer(req.requestId, answer);
        }}
      />

      <Composer
        value={stream.inputVal}
        onChangeText={stream.setInputVal}
        onSend={() => {
          handleScrollToEnd();
          stream.sendMessage();
        }}
        onStop={isAborting ? undefined : handleStop}
        running={stream.running}
      />
    </KeyboardAvoidingView>
  );
}

export default ChatScreen;