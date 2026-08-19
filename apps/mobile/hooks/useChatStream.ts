import { useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ImageAttachment } from "@console/types";
import { sessionService, sessionKeys } from "@console/api";
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
 * Message loading uses a LOCAL `useQuery` (from mobile's own
 * @tanstack/react-query) with the shared `sessionKeys.detail` cache key, so
 * it dedupes with any other `useSession(id)` caller while keeping the hook
 * execution on mobile's React copy. Importing the `useSession` hook from
 * @console/api instead shifted the module graph and triggered a dual-React
 * "Invalid hook call" inside ConsoleApiProvider. The 5-minute staleTime on
 * the QueryClient means re-opening a chat within that window is instant with
 * no network round trip. The store is keyed by sessionId, so switching
 * sessions does NOT wipe the previously loaded messages — the cached state
 * renders instantly and is refreshed in the background when fresh data lands.
 */
export function useChatStream() {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);

  // Single shared fetch for both the message history and the session title.
  // The query key matches @console/api's `sessionKeys.detail`, so this
  // dedupes with any other caller of `useSession(id)` (e.g. the chat header
  // if it ever uses the hook) — one network round trip per session per
  // staleTime window.
  const sessionQuery = useQuery({
    queryKey: sessionKeys.detail(selectedSessionId ?? ""),
    queryFn: () => sessionService.getSession(selectedSessionId!),
    enabled: Boolean(selectedSessionId),
  });

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
    running: snapshot?.running ?? false,
    inputVal: input,
    setInputVal: (text: string) => {
      if (selectedSessionId) setInput(selectedSessionId, text);
    },
    sendMessage: handleSend,
    stop,
    refetchMessages,
    // Title from the shared TanStack Query cache — no separate fetch.
    chatTitle: sessionQuery.data?.header.title ?? "Console",
    // Extra runtime surfaces (desktop parity).
    sessionView,
  };
}
