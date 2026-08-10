import { useCallback, useEffect } from "react";
import type { ImageAttachment } from "@console/types";
import { sessionService } from "@console/api";
import { useAppStore } from "../stores/useAppStore";
import { useChatStore } from "../stores/useChatStore";
import { useSessionStore } from "../stores/useSessionStore";

/**
 * Drives a single chat session's SSE stream against the backend.
 *
 * Re-implemented on top of the desktop-parity `useChatStore` runtime: it
 * reads the derived snapshot for the *selected* session, sends messages
 * through the store's `sendMessage`, and exposes the same surface the mobile
 * chat screen already consumes (messages, streaming buffers, pending items,
 * input value, send/stop/refetch).
 */
export function useChatStream() {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);

  // Derive the UI snapshot for the selected session.
  const snapshot = useChatStore(
    useCallback(
      (state) => (selectedSessionId ? state.getSnapshot(selectedSessionId) : undefined),
      [selectedSessionId],
    ),
  );

  const input = useChatStore(
    useCallback(
      (state) => (selectedSessionId ? state.getSession(selectedSessionId).input : ""),
      [selectedSessionId],
    ),
  );

  const sendMessage = useChatStore((state) => state.sendMessage);
  const abort = useChatStore((state) => state.abort);
  const setInput = useChatStore((state) => state.setInput);
  const loadMessages = useChatStore((state) => state.loadMessages);
  const reset = useChatStore((state) => state.reset);

  // Reset + load persisted messages when the selected session changes.
  useEffect(() => {
    if (selectedSessionId) {
      reset(selectedSessionId);
      fetchSessionMessages(selectedSessionId).catch(() => {});
    }
  }, [selectedSessionId, reset, loadMessages]);

  const fetchSessionMessages = useCallback(
    async (sessionId: string) => {
      if (!sessionId) return;
      try {
        const detail = await sessionService.getSession(sessionId);
        loadMessages(sessionId, detail.messages);
      } catch (e) {
        console.error("Failed to load session messages:", e);
      }
    },
    [loadMessages],
  );

  const handleSend = useCallback(
    async (attachments?: ImageAttachment[]) => {
      if (!selectedSessionId) return;
      if (attachments && attachments.length > 0) {
        useChatStore.getState().addAttachments(selectedSessionId, attachments);
      }
      await sendMessage(selectedSessionId);
    },
    [selectedSessionId, sendMessage],
  );

  const stop = useCallback(() => {
    if (selectedSessionId) {
      void abort(selectedSessionId);
    }
  }, [selectedSessionId, abort]);

  const refetchMessages = useCallback(() => {
    if (selectedSessionId) {
      return fetchSessionMessages(selectedSessionId);
    }
    return Promise.resolve();
  }, [selectedSessionId, fetchSessionMessages]);

  const sessionView = selectedSessionId
    ? useSessionStore.getState().getSession(selectedSessionId)
    : null;

  return {
    messages: snapshot?.messages ?? [],
    streamingText: snapshot?.streamingText ?? "",
    streamingThinking: snapshot?.streamingThinking ?? "",
    activeToolCalls: snapshot?.activeToolCalls ?? [],
    liveToolResults: snapshot?.liveToolResults ?? [],
    pendingPermission: snapshot?.pendingPermission ?? null,
    pendingQuestion: snapshot?.pendingQuestion ?? null,
    running: snapshot?.running ?? false,
    inputVal: input,
    setInputVal: (text: string) => {
      if (selectedSessionId) setInput(selectedSessionId, text);
    },
    sendMessage: handleSend,
    stop,
    refetchMessages,
    // Extra runtime surfaces (desktop parity).
    sessionView,
  };
}
