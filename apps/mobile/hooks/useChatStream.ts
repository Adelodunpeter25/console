import { useState, useEffect, useCallback, useRef } from "react";
import { AgentMessage, ToolCall, ToolResult } from "@console/types";
import { useAppStore } from "../stores/useAppStore";

export function useChatStream() {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);

  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [inputVal, setInputVal] = useState("");
  const [running, setRunning] = useState(false);
  
  // Real-time streaming states matching desktop page
  const [streamingText, setStreamingText] = useState("");
  const [streamingThinking, setStreamingThinking] = useState("");
  const [activeToolCalls, setActiveToolCalls] = useState<ToolCall[]>([]);
  const [liveToolResults, setLiveToolResults] = useState<ToolResult[]>([]);

  // Use refs to avoid closure stale values in XMLHttpRequest handlers
  const streamingTextRef = useRef("");
  const streamingThinkingRef = useRef("");
  const activeToolCallsRef = useRef<ToolCall[]>([]);
  const liveToolResultsRef = useRef<ToolResult[]>([]);

  const fetchSessionMessages = useCallback(async () => {
    if (!selectedSessionId || !backendUrl) return;
    try {
      const response = await fetch(`${backendUrl}/api/sessions/${selectedSessionId}`);
      const data = await response.json();
      if (data && data.data && data.data.messages) {
        setMessages(data.data.messages);
      } else if (data && data.messages) {
        setMessages(data.messages);
      }
    } catch (e) {
      console.error("Failed to load session messages:", e);
    }
  }, [backendUrl, selectedSessionId]);

  useEffect(() => {
    if (selectedSessionId) {
      fetchSessionMessages();
      // Reset streaming states
      setStreamingText("");
      setStreamingThinking("");
      setActiveToolCalls([]);
      setLiveToolResults([]);
      streamingTextRef.current = "";
      streamingThinkingRef.current = "";
      activeToolCallsRef.current = [];
      liveToolResultsRef.current = [];
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
    
    // Reset streaming states before starting
    setStreamingText("");
    setStreamingThinking("");
    setActiveToolCalls([]);
    setLiveToolResults([]);
    streamingTextRef.current = "";
    streamingThinkingRef.current = "";
    activeToolCallsRef.current = [];
    liveToolResultsRef.current = [];

    let xhr: XMLHttpRequest | null = null;

    try {
      let offset = 0;
      let buffer = "";

      xhr = new XMLHttpRequest();
      xhr.open("POST", `${backendUrl}/api/sessions/${selectedSessionId}/run`);
      xhr.setRequestHeader("Content-Type", "application/json");

      const handleChunk = (chunk: string) => {
        buffer += chunk;
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          const trimmed = line.trim();
          if (trimmed.startsWith("data: ")) {
            try {
              const eventData = trimmed.slice(6);
              const event = JSON.parse(eventData);

              switch (event.type) {
                case "modelStreamPart": {
                  const text = event.part?.text;
                  const thinking = event.part?.thinking;
                  if (text) {
                    streamingTextRef.current += text;
                    setStreamingText(streamingTextRef.current);
                  }
                  if (thinking) {
                    streamingThinkingRef.current += thinking;
                    setStreamingThinking(streamingThinkingRef.current);
                  }
                  break;
                }
                case "modelStreamEnd": {
                  const finalMessage: AgentMessage = event.turn || {
                    role: "assistant",
                    content: [
                      ...(streamingThinkingRef.current
                        ? [{ type: "thinking" as const, text: streamingThinkingRef.current }]
                        : []),
                      ...(streamingTextRef.current
                        ? [{ type: "text" as const, text: streamingTextRef.current }]
                        : []),
                    ],
                  };
                  setMessages((prev) => [...prev, finalMessage]);
                  setStreamingText("");
                  setStreamingThinking("");
                  streamingTextRef.current = "";
                  streamingThinkingRef.current = "";
                  break;
                }
                case "toolExecutionStart": {
                  activeToolCallsRef.current = event.calls;
                  setActiveToolCalls(event.calls);
                  setLiveToolResults([]);
                  liveToolResultsRef.current = [];
                  break;
                }
                case "toolExecutionResult": {
                  liveToolResultsRef.current.push(event.result);
                  setLiveToolResults([...liveToolResultsRef.current]);
                  break;
                }
                case "toolExecutionEnd": {
                  const results = event.results || liveToolResultsRef.current;
                  setMessages((prev) => [...prev, { role: "toolResult", results }]);
                  setActiveToolCalls([]);
                  setLiveToolResults([]);
                  activeToolCallsRef.current = [];
                  liveToolResultsRef.current = [];
                  break;
                }
                case "error": {
                  const msg = event.error?.message || "Unknown run error";
                  if (!msg.toLowerCase().includes("aborted")) {
                    setMessages((prev) => [
                      ...prev,
                      { role: "assistant", content: [{ type: "text", text: `Error: ${msg}` }] },
                    ]);
                  }
                  break;
                }
              }
            } catch (e) {
              // Ignore JSON parse errors for incomplete lines
            }
          }
        }
      };

      xhr.onprogress = () => {
        if (!xhr) return;
        const chunk = xhr.responseText.slice(offset);
        offset = xhr.responseText.length;
        handleChunk(chunk);
      };

      xhr.onload = () => {
        setRunning(false);
        fetchSessionMessages();
      };

      xhr.onerror = () => {
        setRunning(false);
        fetchSessionMessages();
      };

      xhr.send(JSON.stringify({ prompt }));
    } catch (err) {
      console.error("SSE run error:", err);
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
    streamingText,
    streamingThinking,
    activeToolCalls,
    liveToolResults,
  };
}
