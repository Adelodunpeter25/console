import { batch, observable } from "@legendapp/state";
import type { AgentMessage, AgentSessionEvent, ImageAttachment } from "@console/types";
import type { ChatSessionState, ChatSnapshot } from "@/types";
import { createChatSessionState, EMPTY_CHAT_SESSION } from "@/types/chat-state";
import { applyChatEvent, toChatSnapshot, ensureMessageIds, newMessageId } from "@/utils/chat-events";
import { reconstructRuns } from "@/utils/reconstruct-runs";
import { provider$ } from "./useProviderStore";
import { app$ } from "./useAppStore";
import { getSession as getSessionView } from "./useSessionStore";
import { registerSessionHasMessagesChecker } from "./useSessionStore";
import { initChatPersistence, setSuppressPersist } from "./chat/chat-persist";
import { trimDraftAttachments } from "./chat/draft";
import {
  updateSession,
  syncSessionStatus,
  randomUUID,
  isAbortError,
  finalizeSessionRun,
  abortSessionStream,
} from "./chat/chat-stream-runner";
import {
  getOrCreateController,
  getController,
  removeController,
  type RunStreamControllerDeps,
} from "./chat/run-stream-controller";
import { answerSessionQuestion, approveSessionPermission } from "./chat/chat-decisions";

/**
 * Chat runtime state (messages, streaming buffers, drafts, runs) as Legend
 * State observables keyed by session id. See
 * docs/legend-state-and-list-migration.md.
 *
 * Reads in components subscribe narrowly via `useValue(chat$.sessions[id])`;
 * imperative reads outside render use `getChatSession(id)`.
 */
export const chat$ = observable({
  sessions: {} as Record<string, ChatSessionState>,
});

// --- Streaming coalescing ---
const _streamBuf: Record<string, { text: string; thinking: string }> = {};
const _streamRaf: Record<string, ReturnType<typeof requestAnimationFrame>> = {};

/** Read one session's state without subscribing (falls back to defaults). */
export function getChatSession(sessionId: string): ChatSessionState {
  return chat$.sessions[sessionId].peek() ?? EMPTY_CHAT_SESSION;
}

export function getChatSnapshot(sessionId: string): ChatSnapshot {
  return toChatSnapshot(getChatSession(sessionId));
}

/** Apply a plain-record updater to the sessions map (one notification per call). */
function setSessions(
  updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>,
): void {
  chat$.sessions.set((prev) => updater(prev));
}

/** Build controller callbacks wired to this module's state helpers. */
function buildControllerDeps(sessionId: string): RunStreamControllerDeps {
  const deps: RunStreamControllerDeps = {
    setSessions,
    handleEvent: (event) => {
      if (event.type === "askQuestion" || event.type === "permissionRequest") {
        syncSessionStatus(sessionId, "needs_attention");
      }
      handleEvent(sessionId, event);
    },
    finalize: (hadError) => {
      finalizeSessionRun(setSessions, sessionId, hadError);
      removeController(sessionId);
    },
    markError: (msg) => markSessionError(sessionId, msg),
    baseUrl: () => app$.backendUrl.peek() ?? "",
  };
  return deps;
}

/** Append an error bubble to the transcript and clear streaming buffers. */
function markSessionError(sessionId: string, msg: string): void {
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (sessionState) => ({
      ...sessionState,
      messages: [
        ...sessionState.messages,
        {
          role: "assistant",
          createdAt: Date.now(),
          content: [{ type: "text", text: `Error: ${msg}` }],
        },
      ],
      streamingText: "",
      streamingThinking: "",
    })),
  );
}

export function loadMessages(sessionId: string, messages: AgentMessage[]): void {
  const current = getChatSession(sessionId);
  if (current.running) return;
  const withIds = ensureMessageIds(messages);
  setSessions((sessions) =>
    updateSession(sessions, sessionId, () => ({
      ...current,
      messages: withIds,
      streamingText: "",
      streamingThinking: "",
      activeToolCalls: [],
      pendingQuestions: [],
      pendingPermissions: [],
      runs: reconstructRuns(messages),
      // Keep draft input/attachments across reloads so image drafts
      // survive going back to home and re-entering the chat.
      // sendMessage and explicit clear/attachment actions manage them.
    })),
  );
}

export function setInput(sessionId: string, value: string): void {
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (current) => ({
      ...current,
      input: value,
      draftUpdatedAt: value.trim().length > 0 || current.attachments.length > 0 ? Date.now() : undefined,
    })),
  );
}

export function setTodoItems(sessionId: string, items: import("@console/types").TodoItem[]): void {
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (current) => ({
      ...current,
      todoItems: items,
    })),
  );
}

export function setSubagents(sessionId: string, subagents: import("@console/types").SubagentInfo[]): void {
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (current) => ({
      ...current,
      subagents,
    })),
  );
}

export function addAttachments(sessionId: string, attachments: ImageAttachment[]): void {
  if (attachments.length === 0) return;
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (current) => {
      const merged = trimDraftAttachments([...current.attachments, ...attachments]);
      return {
        ...current,
        attachments: merged,
        draftUpdatedAt: Date.now(),
      };
    }),
  );
}

export function removeAttachment(sessionId: string, index: number): void {
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (current) => {
      const next = current.attachments.filter((_, i) => i !== index);
      return {
        ...current,
        attachments: next,
        draftUpdatedAt: current.input.trim().length > 0 || next.length > 0 ? Date.now() : undefined,
      };
    }),
  );
}

export function clearAttachments(sessionId: string): void {
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (current) => ({
      ...current,
      attachments: [],
      draftUpdatedAt: current.input.trim().length > 0 ? Date.now() : undefined,
    })),
  );
}

export async function sendMessage(sessionId: string, promptOverride?: string): Promise<void> {
  const session = getChatSession(sessionId);
  const { input, running, attachments } = session;
  const prompt = (promptOverride ?? input).trim();
  if (!prompt || running) return;

  const { sessionModelId, sessionProvider, approvalMode } = getSessionView(sessionId);

  // Validate image support for the selected model.
  if (attachments.length > 0 && sessionModelId && sessionProvider) {
    const selectedModel = provider$
      .modelsByProvider[sessionProvider]
      .peek()
      ?.find((model) => model.id === sessionModelId);
    if (selectedModel?.supportsImages === false) {
      setSessions((sessions) =>
        updateSession(sessions, sessionId, (current) => ({
          ...current,
          messages: [
            ...current.messages,
            {
              role: "assistant",
              createdAt: Date.now(),
              content: [
                {
                  type: "text",
                  text: `Error: The selected model '${sessionModelId}' does not support image attachments.`,
                },
              ],
            },
          ],
        })),
      );
      return;
    }
  }

  setSessions((sessions) =>
    updateSession(sessions, sessionId, (sessionState) => ({
      ...sessionState,
      input: "",
      draftUpdatedAt: undefined,
      running: true,
      messages: [
        ...sessionState.messages,
        {
          role: "user",
          id: newMessageId(),
          content: prompt,
          createdAt: Date.now(),
          ...(attachments.length > 0
            ? {
                attachments: attachments.map((a) => ({ type: "image" as const, ...a })),
              }
            : {}),
        },
      ],
      streamingText: "",
      streamingThinking: "",
      activeToolCalls: [],
      runs: [
        ...sessionState.runs,
        {
          runId: randomUUID(),
          startedAt: Date.now(),
          elapsedMs: 0,
          events: [],
          status: "working" as const,
        },
      ],
      attachments: [],
    })),
  );
  syncSessionStatus(sessionId, "working");

  try {
    setSuppressPersist(true);
    const controller = getOrCreateController(sessionId, buildControllerDeps(sessionId));
    // markError is shared with the controller via deps.
    controller.startRun({
      prompt,
      ...(attachments.length > 0 ? { attachments: session.attachments } : {}),
      ...(sessionModelId ? { modelId: sessionModelId } : {}),
      ...(sessionProvider ? { provider: sessionProvider as import("@console/types").ProviderId } : {}),
      ...(approvalMode ? { approvalMode } : {}),
    });
  } catch (err) {
    const msg =
      err instanceof Error
        ? err.message
        : typeof err === "string"
          ? err
          : "Failed to send message. Is the backend running?";
    if (!isAbortError(msg)) {
      markSessionError(sessionId, msg);
    }
    finalizeSessionRun(setSessions, sessionId, !isAbortError(msg));
    setSuppressPersist(false);
  }
}

export async function abort(sessionId: string): Promise<void> {
  // Kill any live stream + pending reconnect timers before aborting the run.
  getController(sessionId)?.cancel();
  await abortSessionStream(setSessions, sessionId);
}

/**
 * Attach to a server-side active run for this session (re-attach).
 *
 * Called when entering a session whose server header reports "working" but no
 * local stream exists. Loads nothing itself — the chat hook loads persisted
 * messages first — then opens a re-attach stream replaying the current run's
 * buffered events so the transcript converges and stays realtime.
 */
export function attachServerRun(sessionId: string): void {
  const current = getChatSession(sessionId);
  if (current.running || getController(sessionId)) return;

  setSessions((sessions) =>
    updateSession(sessions, sessionId, (sessionState) => {
      const runs = sessionState.runs;
      const hasWorkingRun = runs.length > 0 && runs[runs.length - 1]!.status === "working";
      return {
        ...sessionState,
        running: true,
        runs: hasWorkingRun
          ? runs
          : [
              ...runs,
              {
                runId: randomUUID(),
                startedAt: Date.now(),
                elapsedMs: 0,
                events: [],
                status: "working" as const,
              },
            ],
      };
    }),
  );
  syncSessionStatus(sessionId, "working");

  try {
    setSuppressPersist(true);
    // since=0 replays the whole current run's buffer so tool calls that
    // started before we attached still appear in the timeline.
    getOrCreateController(sessionId, buildControllerDeps(sessionId)).attach(0);
  } catch {
    finalizeSessionRun(setSessions, sessionId, false);
    setSuppressPersist(false);
  }
}

export async function answerQuestion(
  sessionId: string,
  requestId: string,
  answer: string | string[],
): Promise<void> {
  await answerSessionQuestion(setSessions, () => chat$.sessions.peek(), sessionId, requestId, answer);
}

export async function approvePermission(
  sessionId: string,
  requestId: string,
  allow: boolean,
): Promise<void> {
  await approveSessionPermission(setSessions, () => chat$.sessions.peek(), sessionId, requestId, allow);
}

export function clear(sessionId: string): void {
  setSessions((sessions) => updateSession(sessions, sessionId, () => createChatSessionState()));
}

export function reset(sessionId: string): void {
  setSessions((sessions) => updateSession(sessions, sessionId, () => createChatSessionState()));
}

/** Synchronously flush buffered text/thinking (RAF stalls in background). */
export function flushStreamBuffer(sessionId: string): void {
  const pending = _streamBuf[sessionId];
  if (!pending) return;
  if (_streamRaf[sessionId] != null) cancelAnimationFrame(_streamRaf[sessionId]!);
  delete _streamBuf[sessionId];
  delete _streamRaf[sessionId];
  if (!pending.text && !pending.thinking) return;
  setSessions((sessions) =>
    updateSession(sessions, sessionId, (s) => ({
      ...s,
      streamingText: s.streamingText + pending.text,
      streamingThinking: s.streamingThinking + pending.thinking,
    })),
  );
}

export function handleEvent(sessionId: string, event: AgentSessionEvent): void {
  if (event.type === 'modelStreamPart') {
    // Accumulate into buffer
    const buf = _streamBuf[sessionId] ?? { text: '', thinking: '' };
    if (event.part?.text) buf.text += event.part.text;
    if (event.part?.thinking) buf.thinking += event.part.thinking;
    _streamBuf[sessionId] = buf;
    // Cancel previous pending flush
    if (_streamRaf[sessionId] != null) {
      cancelAnimationFrame(_streamRaf[sessionId]!);
    }
    // Schedule a coalesced flush
    _streamRaf[sessionId] = requestAnimationFrame(() => {
      const pending = _streamBuf[sessionId];
      if (!pending) return;
      delete _streamBuf[sessionId];
      delete _streamRaf[sessionId];
      setSessions((sessions) =>
        updateSession(sessions, sessionId, (s) => ({
          ...s,
          streamingText: s.streamingText + pending.text,
          streamingThinking: s.streamingThinking + pending.thinking,
        })),
      );
    });
    return;
  }
  // All other events flush any pending text buffer first, then apply immediately
  batch(() => {
    if (_streamBuf[sessionId]) {
      if (_streamRaf[sessionId] != null) cancelAnimationFrame(_streamRaf[sessionId]!);
      const pending = _streamBuf[sessionId]!;
      delete _streamBuf[sessionId];
      delete _streamRaf[sessionId];
      setSessions((sessions) =>
        updateSession(sessions, sessionId, (s) => ({
          ...s,
          streamingText: s.streamingText + pending.text,
          streamingThinking: s.streamingThinking + pending.thinking,
        })),
      );
    }
    setSessions((sessions) =>
      updateSession(sessions, sessionId, (sessionState) => applyChatEvent(sessionState, event)),
    );
  });
}

// Load persisted sessions once at module startup (before first render). The
// sessions node is passed in so chat-persist doesn't import this module
// (require cycle).
initChatPersistence(chat$.sessions);

// Register checker to avoid cyclic import in useSessionStore
registerSessionHasMessagesChecker((sessionId) => {
  return getChatSession(sessionId).messages.length > 0;
});
