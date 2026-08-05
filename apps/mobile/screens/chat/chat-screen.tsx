import React from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ActivityIndicator,
  KeyboardAvoidingView,
  FlatList,
  Platform,
} from "react-native";
import { AgentMessage } from "@console/types";
import { ScreenHeader } from "../../components/common/screen-header";
import { useChatStream } from "../../hooks";
import { useAppStore } from "../../stores";

export function ChatScreen() {
  const { messages, inputVal, setInputVal, running, sendMessage } = useChatStream();
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={Platform.OS === "ios" ? 90 : 0}
      className="flex-1 pt-4"
      style={{ flex: 1 }}
    >
      <ScreenHeader title="Console Chat" onBack={() => setActiveTab("home")} />

      {messages.length === 0 ? (
        <View className="items-center justify-center py-14">
          <Text className="text-zinc-400 text-sm italic">
            No messages. Type a prompt below to start.
          </Text>
        </View>
      ) : (
        <FlatList
          data={messages}
          keyExtractor={(_, index) => index.toString()}
          contentContainerStyle={{ paddingHorizontal: 16, paddingBottom: 80 }}
          renderItem={({ item }) => {
            const msg = item as AgentMessage;
            const isUser = msg.role === "user";
            let text = "";
            if (msg.role === "user") {
              text = msg.content;
            } else if (msg.role === "assistant") {
              text = msg.content
                .map((c) => (c.type === "text" || c.type === "thinking" ? c.text : ""))
                .join("\n");
            }

            return (
              <View
                className={`p-4 rounded-xl mb-3.5 ${
                  isUser
                    ? "bg-foreground/10 border border-border self-end max-w-[85%]"
                    : "bg-card border border-border self-start w-full max-w-[92%]"
                }`}
              >
                <Text className="text-xs font-bold text-foreground-secondary uppercase mb-1.5">
                  {isUser ? "You" : "Agent"}
                </Text>
                <Text className="text-foreground text-sm leading-6">{text}</Text>
              </View>
            );
          }}
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

export default ChatScreen;
