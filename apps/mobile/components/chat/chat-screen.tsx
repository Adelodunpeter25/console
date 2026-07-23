import React, { useState, useEffect } from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { LegendList } from "@legendapp/list";
import { styles } from "../../styles/styles";

interface ChatScreenProps {
  projectId: string | null;
  sessionId: string | null;
  backendUrl: string;
}

export function ChatScreen({ projectId, sessionId, backendUrl }: ChatScreenProps) {
  const [messages, setMessages] = useState<any[]>([]);
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
    } catch (e) {
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
      setMessages((prev) => [...prev, { role: "assistant", content: [{ type: "text", text: "" }] }]);

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
            } catch (err) {
              // Ignore frames parse issues
            }
          }
        }
      }
    } catch (e) {
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
      style={styles.chatContainer}
    >
      <Text style={styles.headerTitle}>Console Chat</Text>

      <LegendList
        data={messages}
        keyExtractor={(_, index) => index.toString()}
        estimatedItemSize={70}
        contentContainerStyle={styles.messageContentList}
        renderItem={({ item }) => {
          const isUser = item.role === "user";
          let text = "";
          if (typeof item.content === "string") {
            text = item.content;
          } else if (Array.isArray(item.content)) {
            text = item.content
              .map((c: any) => c.text || "")
              .join("\n");
          }

          return (
            <View style={[styles.bubbleContainer, isUser ? styles.bubbleUser : styles.bubbleAgent]}>
              <Text style={styles.bubbleRole}>{isUser ? "You" : "Agent"}</Text>
              <Text style={styles.bubbleText}>{text}</Text>
            </View>
          );
        }}
        ListEmptyComponent={
          <View style={styles.emptyList}>
            <Text style={styles.emptyListText}>No messages. Type a prompt below to start.</Text>
          </View>
        }
      />

      <View style={styles.chatComposer}>
        <TextInput
          style={styles.chatInput}
          value={inputVal}
          onChangeText={setInputVal}
          placeholder="Ask agent to write code..."
          placeholderTextColor="#9095a0"
          multiline
        />
        <TouchableOpacity
          style={[styles.chatSendBtn, (!inputVal.trim() || running) && styles.chatSendBtnDisabled]}
          onPress={handleSend}
          disabled={!inputVal.trim() || running}
        >
          {running ? (
            <ActivityIndicator size="small" color="#09090b" />
          ) : (
            <Text style={styles.chatSendBtnText}>Send</Text>
          )}
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
}
export default ChatScreen;
