import { useCallback, useEffect } from "react";
import type { ImageAttachment } from "@console/types";
import { useSession } from "./queries";
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
 * input value, send/stop).
 *
 * Message loading uses local `useSession(id)` from `./queries`, so
 * it dedupes with any other `useSession(id)` caller while keeping the hook
 * execution on mobile's local React instance.
 */
export function useChatStream() {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);

  // Single shared fetch for both the message history and the session title.
  const sessionQuery = useSession(selectedSessionId ?? "");

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

  // When fresh session data arrives from the server, push the messages into
  // the chat store and sync the session view state (model, cwd, approval mode).
  useEffect(() => {
    if (selectedSessionId && sessionQuery.data) {
      loadMessages(selectedSessionId, sessionQuery.data.messages);

      const header = sessionQuery.data.header;
      useSessionStore.setState((state) => ({
        sessions: {
          ...state.sessions,
          [selectedSessionId]: {
            sessionModelId: header.modelId ?? null,
            sessionProvider: header.provider ?? null,
            sessionCwd: header.cwd ?? null,
            approvalMode: (header.approvalMode as import("@console/types").ApprovalMode) ?? "always-ask",
          },
        },
      }));
    }
  }, [selectedSessionId, sessionQuery.data, loadMessages]);

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
    return sessionQuery.refetch();
  }, [sessionQuery]);

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
    pendingPermissions: snapshot?.pendingPermissions ?? [],
    pendingQuestions: snapshot?.pendingQuestions ?? [],
    running: snapshot?.running ?? false,
    runs: snapshot?.runs ?? [],
    inputVal: input,
    setInputVal: (text: string) => {
      if (selectedSessionId) setInput(selectedSessionId, text);
    },
    sendMessage: handleSend,
    stop,
    refetchMessages,
    isLoadingMessages: sessionQuery.isLoading,
    // Title from the shared TanStack Query cache — no separate fetch.
    chatTitle: sessionQuery.data?.header.title ?? "Console",
    // Extra runtime surfaces (desktop parity).
    sessionView,
  };
}
