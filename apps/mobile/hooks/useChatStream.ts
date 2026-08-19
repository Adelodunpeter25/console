import { useCallback, useEffect } from "react";
import type { ImageAttachment } from "@console/types";
import { useSession } from "@console/api";
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
 * Message loading uses TanStack Query's `useSession` so the fetch is shared
 * with any other caller of `useSession(id)` (e.g. the chat header). The
 * 5-minute staleTime on the QueryClient means re-opening a chat within that
 * window is instant with no network round trip. The store is keyed by
 * sessionId, so switching sessions does NOT wipe the previously loaded
 * messages — the cached state renders instantly and is refreshed in the
 * background when TanStack returns fresh data.
 */
export function useChatStream() {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);

  // Single shared fetch for both the message history and the session title.
  // TanStack Query dedupes this with any other `useSession(id)` caller, so
  // the chat header and the message list never double-fetch.
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
  // the store. We do NOT wipe first — the store keeps whatever it already has
  // (cached from a previous visit or restored from persist) so the UI never
  // blanks. `loadMessages` itself guards against replacing an active run.
  // The `sessionQuery.data` dependency is stable per TanStack's structural
  // sharing, so this only fires when the data genuinely changes.
  useEffect(() => {
    if (selectedSessionId && sessionQuery.data) {
      loadMessages(selectedSessionId, sessionQuery.data.messages);
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
