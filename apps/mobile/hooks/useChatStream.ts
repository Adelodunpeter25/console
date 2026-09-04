import { useCallback, useEffect, useMemo } from "react";
import { AppState } from "react-native";
import type { ImageAttachment } from "@console/types";
import { useInfiniteSession } from "./queries";
import {
  abort as abortChat,
  addAttachments,
  chat$,
  getChatSnapshot,
  loadMessages as loadSessionMessages,
  sendMessage,
  setInput,
} from "@/stores/useChatStore";
import { getSession, sessionsView$ } from "@/stores/useSessionStore";
import { sessionStatuses$ } from "@/stores/useSessionStatusStore";
import {
  attachServerRun,
  flushStreamBuffer,
  getChatSession,
} from "@/stores/useChatStore";
import { getController } from "@/stores/chat/run-stream-controller";
import { app$, setSelectedProjectId } from "@/stores/useAppStore";
import { project$ } from "@/stores/useProjectStore";
import { useValue } from "@legendapp/state/react";

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
  const selectedSessionId = useValue(app$.selectedSessionId);

  // Infinite shared fetch for paginating messages backwards.
  const sessionQuery = useInfiniteSession(selectedSessionId ?? "", 100);

  // Derive the UI snapshot for the selected session.
  const snapshot = useValue(() =>
    selectedSessionId ? getChatSnapshot(selectedSessionId) : undefined,
  );

  const input = useValue(() =>
    selectedSessionId ? chat$.sessions[selectedSessionId].input.get() ?? "" : "",
  );

  // Flatten all pages in ascending chronological order
  const allMessages = useMemo(() => {
    if (!sessionQuery.data?.pages || sessionQuery.data.pages.length === 0) return null;
    return [...sessionQuery.data.pages].reverse().flatMap((p) => p.messages);
  }, [sessionQuery.data?.pages]);

  // Compute a stable fingerprint of messages to prevent unnecessary reload cascades
  const messagesFingerprint = useMemo(() => {
    if (!allMessages) return "";
    const firstId = (allMessages[0] as any)?.id ?? "";
    const lastId = (allMessages[allMessages.length - 1] as any)?.id ?? "";
    return `${allMessages.length}:${firstId}:${lastId}`;
  }, [allMessages]);

  // Header metadata from the latest page
  const latestHeader = sessionQuery.data?.pages[0]?.header;

  // When fresh session data arrives from the server, push the messages into
  // the chat store and sync the session view state (model, cwd, approval mode).
  // Skip loadMessages if the chat session is currently actively streaming/running.
  //
  // Attach-on-entry: when the server reports this session as "working" but no
  // local stream exists, load persisted history then open a re-attach stream
  // (since=0 replays the active run's buffer) so the transcript converges and
  // stays realtime instead of freezing at a snapshot.
  useEffect(() => {
    if (!selectedSessionId || !allMessages) return;

    const isRunning = chat$.sessions[selectedSessionId].running.peek();
    if (!isRunning) {
      loadSessionMessages(selectedSessionId, allMessages);
    }

    const serverStatus = sessionStatuses$[selectedSessionId].peek();
    const hasLocalStream = getController(selectedSessionId);
    if (!isRunning && serverStatus === "working" && !hasLocalStream && selectedSessionId) {
      attachServerRun(selectedSessionId);
    }

    if (latestHeader) {
      const current = sessionsView$[selectedSessionId].peek();
      const nextModel = latestHeader.modelId ?? null;
      const nextProvider = latestHeader.provider ?? null;
      const nextCwd = latestHeader.cwd ?? null;
      const nextApproval =
        (latestHeader.approvalMode as import("@console/types").ApprovalMode) ?? "always-ask";

      const unchanged =
        current &&
        current.sessionModelId === nextModel &&
        current.sessionProvider === nextProvider &&
        current.sessionCwd === nextCwd &&
        current.approvalMode === nextApproval;

      if (!unchanged) {
        sessionsView$[selectedSessionId].set({
          sessionModelId: nextModel,
          sessionProvider: nextProvider,
          sessionCwd: nextCwd,
          approvalMode: nextApproval,
        });
        
        // Also update selectedProjectId if the cwd changed and matches a known project
        // This ensures file browser and terminal pick up the new directory
        if (nextCwd && nextCwd !== current?.sessionCwd) {
          const { projects } = project$.peek();
          const matchingProject = projects.find((p) => p.path === nextCwd);
          if (matchingProject && matchingProject.id !== app$.selectedProjectId.peek()) {
            setSelectedProjectId(matchingProject.id);
          }
        }
      }
    }
  }, [selectedSessionId, messagesFingerprint, latestHeader, loadSessionMessages, allMessages]);

  // Foreground resume: RAF stalls in background so buffered text never
  // flushes, and a stalled native socket emits no onError to trigger the
  // controller reconnect. On active, flush buffers and resume from lastSeq.
  useEffect(() => {
    if (!selectedSessionId) return;
    const sub = AppState.addEventListener("change", (status) => {
      if (status !== "active") return;
      const id = selectedSessionId;
      flushStreamBuffer(id);
      const running = getChatSession(id).running;
      const controller = getController(id);
      if (running && controller?.isActive) {
        controller.attach(controller.lastSeqValue);
      }
    });
    return () => sub.remove();
  }, [selectedSessionId]);

  const handleSend = useCallback(
    async (attachments?: ImageAttachment[]) => {
      if (!selectedSessionId) return;
      if (attachments && attachments.length > 0) {
        addAttachments(selectedSessionId, attachments);
      }
      await sendMessage(selectedSessionId);
    },
    [selectedSessionId, sendMessage],
  );

  const stop = useCallback(() => {
    if (selectedSessionId) {
      void abortChat(selectedSessionId);
    }
  }, [selectedSessionId]);

  const refetchMessages = useCallback(() => {
    return sessionQuery.refetch();
  }, [sessionQuery]);

  const fetchEarlierMessages = useCallback(() => {
    if (sessionQuery.hasNextPage && !sessionQuery.isFetchingNextPage) {
      void sessionQuery.fetchNextPage();
    }
  }, [sessionQuery]);

  const sessionView = selectedSessionId
    ? getSession(selectedSessionId)
    : null;

  const effectiveMessages = useMemo(() => {
    if (snapshot?.messages && snapshot.messages.length > 0) {
      return snapshot.messages;
    }
    return allMessages ?? [];
  }, [snapshot?.messages, allMessages]);

  return {
    messages: effectiveMessages,
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
    isLoadingMessages: sessionQuery.isLoading && effectiveMessages.length === 0,
    // Title from the shared TanStack Query cache — no separate fetch.
    chatTitle: latestHeader?.title ?? "Console",
    // Extra runtime surfaces (desktop parity).
    sessionView,
  };
}
