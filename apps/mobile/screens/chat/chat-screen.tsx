import React, { useRef } from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ActivityIndicator,
  FlatList,
} from "react-native";
import { KeyboardAvoidingView } from "react-native-keyboard-controller";
import { AgentMessage, ToolResult } from "@console/types";
import { ScreenHeader } from "../../components/common/screen-header";
import { MarkdownRenderer } from "../../components/common/markdown-renderer";
import { useChatStream } from "../../hooks";
import { useAppStore } from "../../stores";
import { theme } from "../../styles/theme";

function ToolResultItem({ result }: { result: ToolResult }) {
  const isError = result.isError;
  return (
    <View className="bg-card-alt border border-border rounded-lg p-2.5 mb-1.5">
      <View className="flex-row justify-between items-center mb-1">
        <Text className="text-xs font-mono font-bold text-foreground">⚙️ {result.toolName}</Text>
        <Text
          className="text-[9px] font-bold font-mono tracking-wide"
          style={{ color: isError ? theme.colors.status.attention : theme.colors.status.ready }}
        >
          {isError ? "FAILED" : "DONE"}
        </Text>
      </View>
      <Text className="text-xs font-mono text-foreground-secondary leading-4" numberOfLines={3} selectable>
        {String(result.content || "")}
      </Text>
    </View>
  );
}

function UserBubble({ content }: { content: string }) {
  return (
    <View className="bg-foreground/5 border border-border rounded-xl p-3.5 mb-3.5 self-end max-w-[85%]">
      <Text className="text-[9px] font-mono font-bold text-foreground-secondary mb-2 tracking-widest">YOU</Text>
      <Text className="text-foreground text-sm leading-5" selectable>{content}</Text>
    </View>
  );
}

function AssistantBubble({
  textContent,
  thinkingContent,
  label = "AGENT",
  toolCalls,
  isStreaming,
}: {
  textContent?: string;
  thinkingContent?: string;
  label?: string;
  toolCalls?: { name: string }[];
  isStreaming?: boolean;
}) {
  return (
    <View className="bg-card border border-border rounded-xl p-3.5 mb-3.5 self-start w-full max-w-[92%]">
      <Text className="text-[9px] font-mono font-bold text-foreground-secondary mb-2 tracking-widest">
        {label}
      </Text>
      {thinkingContent ? (
        <View className="border-l-2 border-foreground-secondary/30 pl-2.5 mb-3">
          <Text className="text-[11px] font-mono text-foreground-secondary mb-1">💭 Thinking...</Text>
          <Text className="text-[11px] font-mono text-foreground-secondary/70 leading-[18px]">
            {thinkingContent}
          </Text>
        </View>
      ) : null}
      {textContent ? <MarkdownRenderer content={textContent} /> : null}
      {toolCalls && toolCalls.length > 0 ? (
        <View className="mt-2 pt-2 border-t border-border">
          {toolCalls.map((call, idx) => (
            <View key={idx} className="flex-row items-center my-1">
              <ActivityIndicator size="small" color={theme.colors.status.running} style={{ marginRight: 8 }} />
              <Text className="text-[11px] font-mono text-foreground-secondary">
                Running {call.name}…
              </Text>
            </View>
          ))}
        </View>
      ) : null}
      {isStreaming && !textContent && !thinkingContent && (!toolCalls || toolCalls.length === 0) ? (
        <ActivityIndicator size="small" color={theme.colors.text.muted} />
      ) : null}
    </View>
  );
}

export function ChatScreen() {
  const {
    messages,
    inputVal,
    setInputVal,
    running,
    sendMessage,
    streamingText,
    streamingThinking,
    activeToolCalls,
  } = useChatStream();

  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const flatListRef = useRef<FlatList>(null);

  const isStreaming = running && (!!streamingText || !!streamingThinking || activeToolCalls.length > 0);

  const renderItem = ({ item, index }: { item: AgentMessage; index: number }) => {
    if (item.role === "user") {
      return <UserBubble content={item.content} />;
    }

    if (item.role === "toolResult") {
      return (
        <View className="w-full max-w-[92%] mb-3.5 self-start">
          {item.results.map((res, i) => (
            <ToolResultItem key={i} result={res} />
          ))}
        </View>
      );
    }

    const textContent = item.content
      .filter((c) => c.type === "text")
      .map((c) => (c.type === "text" ? c.text : ""))
      .join("\n\n");

    const thinkingContent = item.content
      .filter((c) => c.type === "thinking")
      .map((c) => (c.type === "thinking" ? c.text : ""))
      .join("\n\n");

    return <AssistantBubble textContent={textContent} thinkingContent={thinkingContent} />;
  };

  return (
    <KeyboardAvoidingView className="flex-1" behavior="padding">
      <ScreenHeader title="Console" onBack={() => setActiveTab("home")} />

      {messages.length === 0 && !isStreaming ? (
        <View className="flex-1 items-center justify-center">
          <Text className="text-foreground-secondary text-sm italic">
            No messages. Type a prompt below to start.
          </Text>
        </View>
      ) : (
        <FlatList
          ref={flatListRef}
          data={messages}
          keyExtractor={(_, i) => i.toString()}
          contentContainerStyle={{ paddingHorizontal: 16, paddingBottom: 16 }}
          onContentSizeChange={() => flatListRef.current?.scrollToEnd({ animated: true })}
          renderItem={renderItem}
          ListFooterComponent={
            isStreaming ? (
              <AssistantBubble
                label="AGENT (STREAMING)"
                textContent={streamingText}
                thinkingContent={streamingThinking}
                toolCalls={activeToolCalls}
                isStreaming
              />
            ) : null
          }
        />
      )}

      {/* Composer */}
      <View className="flex-row gap-2.5 px-4 py-3 bg-screen border-t border-border items-end">
        <TextInput
          className="flex-1 min-h-11 max-h-28 bg-card border border-border rounded-2xl px-4 py-2.5 text-foreground text-sm"
          value={inputVal}
          onChangeText={setInputVal}
          placeholder="Ask agent to write code..."
          placeholderTextColor="#71717a"
          multiline
        />
        <TouchableOpacity
          className={`w-11 h-11 bg-foreground rounded-full items-center justify-center ${
            !inputVal.trim() || running ? "opacity-30" : ""
          }`}
          onPress={sendMessage}
          disabled={!inputVal.trim() || running}
        >
          {running ? (
            <ActivityIndicator size="small" color="#000000" />
          ) : (
            <Text className="text-black text-sm font-bold">↑</Text>
          )}
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
}

export default ChatScreen;
