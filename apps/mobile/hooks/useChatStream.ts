import { useCallback, useEffect, useMemo } from "react";
import type { ImageAttachment } from "@console/types";
import { useInfiniteSession } from "./queries";
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
 * input value, send/stop, backward pagination).
 *
 * Message loading uses local `useInfiniteSession(id)` from `./queries`, so
 * older message batches are loaded as the user scrolls up.
 */
export function useChatStream() {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);

  // Infinite shared fetch for paginating messages backwards.
  const sessionQuery = useInfiniteSession(selectedSessionId ?? "", 100);

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

  // Flatten all pages in ascending chronological order
  const allMessages = useMemo(() => {
    if (!sessionQuery.data?.pages || sessionQuery.data.pages.length === 0) return null;
    return [...sessionQuery.data.pages].reverse().flatMap((p) => p.messages);
  }, [sessionQuery.data?.pages]);

  // Header metadata from the latest page
  const latestHeader = sessionQuery.data?.pages[0]?.header;

  // When fresh session data arrives from the server, push the messages into
  // the chat store and sync the session view state (model, cwd, approval mode).
  useEffect(() => {
    if (selectedSessionId && allMessages) {
      loadMessages(selectedSessionId, allMessages);

      if (latestHeader) {
        useSessionStore.setState((state) => ({
          sessions: {
            ...state.sessions,
            [selectedSessionId]: {
              sessionModelId: latestHeader.modelId ?? null,
              sessionProvider: latestHeader.provider ?? null,
              sessionCwd: latestHeader.cwd ?? null,
              approvalMode: (latestHeader.approvalMode as import("@console/types").ApprovalMode) ?? "always-ask",
            },
          },
        }));
      }
    }
  }, [selectedSessionId, allMessages, latestHeader, loadMessages]);

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

  const fetchEarlierMessages = useCallback(() => {
    if (sessionQuery.hasNextPage && !sessionQuery.isFetchingNextPage) {
      void sessionQuery.fetchNextPage();
    }
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
    hasEarlierMessages: Boolean(sessionQuery.hasNextPage),
    isFetchingEarlierMessages: sessionQuery.isFetchingNextPage,
    fetchEarlierMessages,
    isLoadingMessages: sessionQuery.isLoading,
    // Title from the shared TanStack Query cache — no separate fetch.
    chatTitle: latestHeader?.title ?? "Console",
    // Extra runtime surfaces (desktop parity).
    sessionView,
  };
}
