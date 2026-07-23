import React, { useState, useEffect } from "react";
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

interface ChatScreenProps {
  projectId: string | null;
  sessionId: string | null;
  backendUrl: string;
}

export function ChatScreen({ projectId, sessionId, backendUrl }: ChatScreenProps) {
  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [inputVal, setInputVal] = useState("");
  const [running, setRunning] = useState(false);
  const [activeSession, setActiveSession] = useState<string | null>(sessionId);

  useEffect(() => {
    setActiveSession(sessionId);
    if (sessionId) {
      fetchSessionMessages();
    }
  }, [sessionId]);

  const fetchSessionMessages = async () => {
    try {
      const response = await fetch(`${backendUrl}/api/sessions/${sessionId}`);
      const data = await response.json();
      if (data && data.messages) {
        setMessages(data.messages);
      }
    } catch {
      // Ignore initial load fetch errors
    }
  };

  const handleSend = async () => {
    if (!inputVal.trim() || !activeSession) return;
    const prompt = inputVal.trim();
    setInputVal("");
    setRunning(true);

    // Optimistically push user message to UI
    setMessages((prev) => [...prev, { role: "user", content: prompt }]);

    try {
      const res = await fetch(`${backendUrl}/api/sessions/${activeSession}/run`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt }),
      });

      if (!res.body) {
        setRunning(false);
        fetchSessionMessages();
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let accumulatedText = "";
      let buffer = "";

      // Add temporary response placeholder
      setMessages((prev) => [
        ...prev,
        { role: "assistant", content: [{ type: "text", text: "" }] },
      ]);

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          const trimmed = line.trim();
          if (trimmed.startsWith("data: ")) {
            try {
              const frame = JSON.parse(trimmed.slice(6));
              if (frame.type === "modelStreamPart" && frame.part?.text) {
                accumulatedText += frame.part.text;
                setMessages((prev) => {
                  const updated = [...prev];
                  const lastIndex = updated.length - 1;
                  if (updated[lastIndex] && updated[lastIndex].role === "assistant") {
                    updated[lastIndex] = {
                      role: "assistant",
                      content: [{ type: "text", text: accumulatedText }],
                    };
                  }
                  return updated;
                });
              }
            } catch {
              // Ignore frames parse issues
            }
          }
        }
      }
    } catch {
      fetchSessionMessages();
    } finally {
      setRunning(false);
      fetchSessionMessages();
    }
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={Platform.OS === "ios" ? 90 : 0}
      className="flex-1 pt-4"
    >
      <Text className="text-2xl font-bold text-white mb-4 tracking-tight px-4">Console Chat</Text>

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
                    ? "bg-white/10 border border-white/20 self-end max-w-[85%]"
                    : "bg-[#121316] border border-white/10 self-start w-full max-w-[92%]"
                }`}
              >
                <Text className="text-xs font-bold text-zinc-400 uppercase mb-1.5">
                  {isUser ? "You" : "Agent"}
                </Text>
                <Text className="text-white text-sm leading-6">{text}</Text>
              </View>
            );
          }}
        />
      )}

      <View className="flex-row gap-2.5 p-3.5 bg-[#0d0d0e] border-t border-white/10 items-center">
        <TextInput
          className="flex-1 min-h-11 max-h-28 bg-[#16171a] border border-white/20 rounded-xl px-4 py-2.5 text-white text-sm"
          value={inputVal}
          onChangeText={setInputVal}
          placeholder="Ask agent to write code..."
          placeholderTextColor="#71717a"
          multiline
        />
        <TouchableOpacity
          className={`h-11 bg-white rounded-xl px-5 items-center justify-center ${
            !inputVal.trim() || running ? "opacity-30" : ""
          }`}
          onPress={handleSend}
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
