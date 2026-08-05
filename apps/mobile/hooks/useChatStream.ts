import { useCallback, useEffect, useRef, useState } from "react";
import type { ImageAttachment } from "@console/types";
import { useAppStore } from "../stores/useAppStore";
import { useChatStore } from "../stores/useChatStore";
import { createSseParser } from "../utils/sse";
import { applyChatEvent } from "../utils/chat-events";
import type { ChatSnapshot } from "../types";

/**
 * Drives a single chat session's SSE stream against the backend.
 * Events flow through the pure `applyChatEvent` reducer and land in the
 * zustand chat store, so the UI renders from one snapshot source of truth.
 */
export function useChatStream() {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);

  const [inputVal, setInputVal] = useState("");
  const xhrRef = useRef<XMLHttpRequest | null>(null);

  // Selectors into the shared chat store
  const messages = useChatStore((state) => state.messages);
  const streamingText = useChatStore((state) => state.streamingText);
  const streamingThinking = useChatStore((state) => state.streamingThinking);
  const activeToolCalls = useChatStore((state) => state.activeToolCalls);
  const liveToolResults = useChatStore((state) => state.liveToolResults);
  const pendingPermission = useChatStore((state) => state.pendingPermission);
  const pendingQuestion = useChatStore((state) => state.pendingQuestion);
  const running = useChatStore((state) => state.running);

  const setSnapshot = useChatStore((state) => state.setSnapshot);
  const reset = useChatStore((state) => state.reset);

  /** Merge a partial patch into the current store snapshot. */
  const commit = useCallback(
    (patch: Partial<ChatSnapshot>) => {
      const current = useChatStore.getState();
      setSnapshot({ ...current, ...patch });
    },
    [setSnapshot],
  );

  /** Apply a batch of streamed events through the reducer, committing once. */
  const applyEvents = useCallback(
    (events: Parameters<typeof applyChatEvent>[1][]) => {
      if (events.length === 0) return;
      const state = useChatStore.getState();
      let snapshot: ChatSnapshot = {
        messages: state.messages,
        streamingText: state.streamingText,
        streamingThinking: state.streamingThinking,
        activeToolCalls: state.activeToolCalls,
        liveToolResults: state.liveToolResults,
        pendingPermission: state.pendingPermission,
        pendingQuestion: state.pendingQuestion,
        running: state.running,
      };
      for (const event of events) {
        snapshot = applyChatEvent(snapshot, event);
      }
      setSnapshot(snapshot);
    },
    [setSnapshot],
  );

  const fetchSessionMessages = useCallback(async () => {
    if (!selectedSessionId || !backendUrl) return;
    try {
      const response = await fetch(`${backendUrl}/api/sessions/${selectedSessionId}`);
      const data = await response.json();
      const history =
        (data && data.data && data.data.messages) || (data && data.messages) || [];
      commit({ messages: history });
    } catch (e) {
      console.error("Failed to load session messages:", e);
    }
  }, [backendUrl, selectedSessionId, commit]);

  useEffect(() => {
    reset();
    if (selectedSessionId) {
      fetchSessionMessages();
    }
  }, [selectedSessionId, fetchSessionMessages, reset]);

  const sendMessage = useCallback(
    async (attachments?: ImageAttachment[]) => {
      if (!inputVal.trim() || !selectedSessionId || !backendUrl || running) return;

      const prompt = inputVal.trim();
      setInputVal("");

      // Optimistically push the user message, clear streaming state.
      const current = useChatStore.getState();
      setSnapshot({
        ...current,
        messages: [...current.messages, { role: "user", content: prompt }],
        streamingText: "",
        streamingThinking: "",
        activeToolCalls: [],
        liveToolResults: [],
        pendingPermission: null,
        pendingQuestion: null,
        running: true,
      });

      const parser = createSseParser();
      const xhr = new XMLHttpRequest();
      xhrRef.current = xhr;

      xhr.open("POST", `${backendUrl}/api/sessions/${selectedSessionId}/run`);
      xhr.setRequestHeader("Content-Type", "application/json");

      let offset = 0;
      xhr.onprogress = () => {
        if (!xhr) return;
        const chunk = xhr.responseText.slice(offset);
        offset = xhr.responseText.length;
        applyEvents(parser.push(chunk));
      };

      xhr.onload = () => {
        parser.flush();
        commit({ running: false });
        fetchSessionMessages();
      };

      xhr.onerror = () => {
        parser.flush();
        commit({ running: false });
        fetchSessionMessages();
      };

      try {
        xhr.send(JSON.stringify({ prompt, ...(attachments ? { attachments } : {}) }));
      } catch (err) {
        console.error("SSE run error:", err);
        commit({ running: false });
        fetchSessionMessages();
      }
    },
    [
      applyEvents,
      backendUrl,
      commit,
      fetchSessionMessages,
      inputVal,
      running,
      selectedSessionId,
      setSnapshot,
    ],
  );

  /** Cut the local stream without touching server state (server abort via useAbort). */
  const stop = useCallback(() => {
    xhrRef.current?.abort();
    xhrRef.current = null;
    commit({ running: false });
  }, [commit]);

  return {
    messages,
    streamingText,
    streamingThinking,
    activeToolCalls,
    liveToolResults,
    pendingPermission,
    pendingQuestion,
    running,
    inputVal,
    setInputVal,
    sendMessage,
    stop,
    refetchMessages: fetchSessionMessages,
  };
}
