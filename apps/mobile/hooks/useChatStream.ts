import { useState, useEffect, useCallback } from "react";
import { AgentMessage } from "@console/types";
import { useAppStore } from "../stores/useAppStore";

export function useChatStream() {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);

  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [inputVal, setInputVal] = useState("");
  const [running, setRunning] = useState(false);

  const fetchSessionMessages = useCallback(async () => {
    if (!selectedSessionId || !backendUrl) return;
    try {
      const response = await fetch(`${backendUrl}/api/sessions/${selectedSessionId}`);
      const data = await response.json();
      if (data && data.messages) {
        setMessages(data.messages);
      }
    } catch {
      // Ignore load errors
    }
  }, [backendUrl, selectedSessionId]);

  useEffect(() => {
    if (selectedSessionId) {
      fetchSessionMessages();
    } else {
      setMessages([]);
    }
  }, [selectedSessionId, fetchSessionMessages]);

  const sendMessage = useCallback(async () => {
    if (!inputVal.trim() || !selectedSessionId || !backendUrl || running) return;

    const prompt = inputVal.trim();
    setInputVal("");
    setRunning(true);

    // Optimistically push user message to UI
    setMessages((prev) => [...prev, { role: "user", content: prompt }]);

    try {
      const res = await fetch(`${backendUrl}/api/sessions/${selectedSessionId}/run`, {
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

      // Temporary assistant response placeholder
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
  }, [backendUrl, fetchSessionMessages, inputVal, running, selectedSessionId]);

  return {
    messages,
    inputVal,
    setInputVal,
    running,
    sendMessage,
    refetchMessages: fetchSessionMessages,
  };
}
