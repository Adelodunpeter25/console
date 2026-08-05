import React, { useRef } from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ActivityIndicator,
  KeyboardAvoidingView,
  FlatList,
  Platform,
  StyleSheet,
} from "react-native";
import { AgentMessage, ToolCall, ToolResult } from "@console/types";
import { ScreenHeader } from "../../components/common/screen-header";
import { MarkdownRenderer } from "../../components/common/markdown-renderer";
import { useChatStream } from "../../hooks";
import { useAppStore } from "../../stores";
import { theme } from "../../styles/theme";

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
    liveToolResults,
  } = useChatStream();
  
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const flatListRef = useRef<FlatList>(null);

  // Combine messages with the live stream if currently active
  const renderMessages = [...messages];
  const isStreaming = running && (streamingText || streamingThinking || activeToolCalls.length > 0);

  const renderToolResult = (result: ToolResult, index: number) => {
    const isError = result.isError;
    return (
      <View key={index} style={styles.toolResultItem}>
        <View style={styles.toolHeader}>
          <Text style={styles.toolName}>⚙️ {result.toolName}</Text>
          <Text style={isError ? styles.toolStatusError : styles.toolStatusOk}>
            {isError ? "FAILED" : "COMPLETED"}
          </Text>
        </View>
        <Text style={styles.toolContent} numberOfLines={3} selectable>
          {String(result.content || "")}
        </Text>
      </View>
    );
  };

  const renderMessageItem = (msg: AgentMessage, index: number) => {
    const isUser = msg.role === "user";
    
    if (isUser) {
      return (
        <View key={index} style={styles.userBubble}>
          <Text style={styles.bubbleAuthor}>YOU</Text>
          <Text style={styles.userText}>{msg.content}</Text>
        </View>
      );
    }

    if (msg.role === "toolResult") {
      return (
        <View key={index} style={styles.toolResultBlock}>
          {msg.results.map((res, idx) => renderToolResult(res, idx))}
        </View>
      );
    }

    // Assistant message
    const textContent = msg.content
      .filter((c) => c.type === "text")
      .map((c) => (c.type === "text" ? c.text : ""))
      .join("\n\n");

    const thinkingContent = msg.content
      .filter((c) => c.type === "thinking")
      .map((c) => (c.type === "thinking" ? c.text : ""))
      .join("\n\n");

    return (
      <View key={index} style={styles.assistantBubble}>
        <Text style={styles.bubbleAuthor}>AGENT</Text>
        {thinkingContent ? (
          <View style={styles.thinkingBlock}>
            <Text style={styles.thinkingTitle}>💭 Thinking Process</Text>
            <Text style={styles.thinkingText}>{thinkingContent}</Text>
          </View>
        ) : null}
        {textContent ? (
          <MarkdownRenderer content={textContent} />
        ) : null}
      </View>
    );
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={Platform.OS === "ios" ? 90 : 0}
      className="flex-1 pt-4"
      style={{ flex: 1 }}
    >
      <ScreenHeader title="Console Chat" onBack={() => setActiveTab("home")} />

      {messages.length === 0 && !isStreaming ? (
        <View className="items-center justify-center py-14">
          <Text className="text-foreground-secondary text-sm italic">
            No messages. Type a prompt below to start.
          </Text>
        </View>
      ) : (
        <FlatList
          ref={flatListRef}
          data={renderMessages}
          keyExtractor={(_, index) => index.toString()}
          contentContainerStyle={{ paddingHorizontal: 16, paddingBottom: 120 }}
          onContentSizeChange={() => flatListRef.current?.scrollToEnd({ animated: true })}
          renderItem={({ item, index }) => renderMessageItem(item, index)}
          ListFooterComponent={
            isStreaming ? (
              <View style={styles.assistantBubble}>
                <Text style={styles.bubbleAuthor}>AGENT (STREAMING)</Text>
                {streamingThinking ? (
                  <View style={styles.thinkingBlock}>
                    <Text style={styles.thinkingTitle}>💭 Thinking...</Text>
                    <Text style={styles.thinkingText}>{streamingThinking}</Text>
                  </View>
                ) : null}
                {streamingText ? (
                  <MarkdownRenderer content={streamingText} />
                ) : null}
                {activeToolCalls.length > 0 ? (
                  <View style={styles.toolLoadingBlock}>
                    {activeToolCalls.map((call, idx) => (
                      <View key={idx} style={styles.toolLoadingItem}>
                        <ActivityIndicator size="small" color={theme.colors.status.running} style={{ marginRight: 8 }} />
                        <Text style={styles.toolLoadingText}>Running tool: {call.name}...</Text>
                      </View>
                    ))}
                  </View>
                ) : null}
              </View>
            ) : null
          }
        />
      )}

      <View className="flex-row gap-2.5 p-3.5 bg-screen border-t border-border items-center">
        <TextInput
          className="flex-1 min-h-11 max-h-28 bg-card border border-border rounded-xl px-4 py-2.5 text-foreground text-sm"
          value={inputVal}
          onChangeText={setInputVal}
          placeholder="Ask agent to write code..."
          placeholderTextColor="#71717a"
          multiline
        />
        <TouchableOpacity
          className={`h-11 bg-foreground rounded-xl px-5 items-center justify-center ${
            !inputVal.trim() || running ? "opacity-30" : ""
          }`}
          onPress={sendMessage}
          disabled={!inputVal.trim() || running}
        >
          {running ? (
            <ActivityIndicator size="small" color="#000000" />
          ) : (
            <Text className="text-black text-sm font-bold">Send</Text>
          )}
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  userBubble: {
    backgroundColor: "rgba(255, 255, 255, 0.06)",
    borderColor: theme.colors.border,
    borderWidth: 1,
    borderRadius: theme.roundness.md,
    padding: 14,
    marginBottom: 14,
    alignSelf: "flex-end",
    maxWidth: "85%",
  },
  assistantBubble: {
    backgroundColor: theme.colors.surface,
    borderColor: theme.colors.border,
    borderWidth: 1,
    borderRadius: theme.roundness.md,
    padding: 14,
    marginBottom: 14,
    alignSelf: "flex-start",
    width: "100%",
    maxWidth: "92%",
  },
  bubbleAuthor: {
    fontSize: 9,
    fontFamily: theme.fonts.monoBold,
    color: theme.colors.text.muted,
    marginBottom: 8,
    letterSpacing: 1,
  },
  userText: {
    color: theme.colors.text.primary,
    fontSize: 14,
    lineHeight: 20,
  },
  thinkingBlock: {
    borderLeftColor: theme.colors.text.muted,
    borderLeftWidth: 2,
    paddingLeft: 10,
    marginBottom: 12,
  },
  thinkingTitle: {
    fontSize: 11,
    fontFamily: theme.fonts.monoMedium,
    color: theme.colors.text.secondary,
    marginBottom: 4,
  },
  thinkingText: {
    fontSize: 12,
    fontFamily: theme.fonts.mono,
    color: theme.colors.text.muted,
    lineHeight: 18,
  },
  toolResultBlock: {
    width: "100%",
    maxWidth: "92%",
    marginBottom: 14,
    alignSelf: "flex-start",
  },
  toolResultItem: {
    backgroundColor: theme.colors.surfaceElevated,
    borderColor: theme.colors.border,
    borderWidth: 1,
    borderRadius: theme.roundness.sm,
    padding: 10,
    marginBottom: 6,
  },
  toolHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 4,
  },
  toolName: {
    fontSize: 11,
    fontFamily: theme.fonts.monoBold,
    color: theme.colors.text.primary,
  },
  toolStatusOk: {
    fontSize: 9,
    fontFamily: theme.fonts.monoBold,
    color: theme.colors.status.ready,
  },
  toolStatusError: {
    fontSize: 9,
    fontFamily: theme.fonts.monoBold,
    color: theme.colors.status.attention,
  },
  toolContent: {
    fontSize: 11,
    fontFamily: theme.fonts.mono,
    color: theme.colors.text.secondary,
    lineHeight: 16,
  },
  toolLoadingBlock: {
    marginTop: 8,
    paddingTop: 8,
    borderTopColor: theme.colors.border,
    borderTopWidth: 1,
  },
  toolLoadingItem: {
    flexDirection: "row",
    alignItems: "center",
    marginVertical: 4,
  },
  toolLoadingText: {
    fontSize: 11,
    fontFamily: theme.fonts.mono,
    color: theme.colors.text.secondary,
  },
});

export default ChatScreen;
