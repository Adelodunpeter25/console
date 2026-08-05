import React, { useRef } from "react";
import { View, Text, FlatList } from "react-native";
import { KeyboardAvoidingView } from "react-native-keyboard-controller";
import { useChatStream, useAbort, useChatDecisions } from "../../hooks";
import { useAppStore } from "../../stores";
import { ScreenHeader } from "../../components/layout/screen-header";
import { MessageBubble } from "../../components/chat/message-bubbles";
import { LiveToolResults } from "../../components/chat/live-tool-results";
import { Composer } from "../../components/chat/composer";
import { ApprovalPanel } from "../../components/chat/approval-panel";

export function ChatScreen() {
  const stream = useChatStream();
  const { abort, isAborting } = useAbort();
  const decisions = useChatDecisions();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const flatListRef = useRef<FlatList>(null);

  const isStreaming =
    stream.running &&
    (!!stream.streamingText || !!stream.streamingThinking || stream.activeToolCalls.length > 0);

  const handleStop = () => {
    stream.stop();
    abort();
  };

  const renderItem = ({ item, index }: { item: (typeof stream.messages)[number]; index: number }) => (
    <MessageBubble key={index} item={item} />
  );

  return (
    <KeyboardAvoidingView className="flex-1" behavior="padding">
      <ScreenHeader title="Console" onBack={() => setActiveTab("home")} />

      {stream.messages.length === 0 && !isStreaming ? (
        <View className="flex-1 items-center justify-center">
          <Text className="text-foreground-secondary text-sm italic">
            No messages. Type a prompt below to start.
          </Text>
        </View>
      ) : (
        <FlatList
          ref={flatListRef}
          data={stream.messages}
          keyExtractor={(_, i) => i.toString()}
          contentContainerStyle={{ paddingHorizontal: 16, paddingBottom: 16 }}
          onContentSizeChange={() => flatListRef.current?.scrollToEnd({ animated: true })}
          renderItem={renderItem}
          ListFooterComponent={
            <View>
              <LiveToolResults results={stream.liveToolResults} />
              {isStreaming ? (
                <MessageBubble
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
        onSend={() => stream.sendMessage()}
        onStop={isAborting ? undefined : handleStop}
        running={stream.running}
      />
    </KeyboardAvoidingView>
  );
}

export default ChatScreen;
