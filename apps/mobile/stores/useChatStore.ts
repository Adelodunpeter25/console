import { batch, observable } from "@legendapp/state";
import type { AgentMessage, AgentSessionEvent, ImageAttachment } from "@console/types";
import type { ChatSessionState, ChatSnapshot } from "@/types";
import { createChatSessionState, EMPTY_CHAT_SESSION } from "@/types/chat-state";
import { applyChatEvent, toChatSnapshot } from "@/utils/chat-events";
import { reconstructRuns } from "@/utils/reconstruct-runs";
import { startNativeChatStream } from "@/utils/native-stream";
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

export function loadMessages(sessionId: string, messages: AgentMessage[]): void {
  const current = getChatSession(sessionId);
  if (current.running) return;
  setSessions((sessions) =>
    updateSession(sessions, sessionId, () => ({
      ...current,
      messages,
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

  const markError = (msg: string) => {
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
  };

  try {
    const baseUrl = app$.backendUrl.peek() ?? "";
    let hadError = false;

    setSuppressPersist(true);
    startNativeChatStream(
      `chat-${sessionId}-${Date.now()}`,
      `${baseUrl}/api/sessions/${sessionId}/run`,
      {
        prompt,
        ...(attachments.length > 0 ? { attachments: session.attachments } : {}),
        ...(sessionModelId ? { modelId: sessionModelId } : {}),
        ...(sessionProvider ? { provider: sessionProvider } : {}),
        ...(approvalMode ? { approvalMode } : {}),
      },
      {
        onEvent: (event) => {
          if (event.type === "error" && !isAbortError(event.error.message)) {
            hadError = true;
          }
          if (event.type === "askQuestion" || event.type === "permissionRequest") {
            syncSessionStatus(sessionId, "needs_attention");
          }
          handleEvent(sessionId, event);
        },
        onError: (errMsg) => {
          if (!isAbortError(errMsg)) {
            hadError = true;
            markError(errMsg);
          }
          setSuppressPersist(false);
        },
        onEnd: (aborted) => {
          finalizeSessionRun(setSessions, sessionId, hadError && !aborted);
          setSuppressPersist(false);
        },
      },
    );
  } catch (err) {
    const msg =
      err instanceof Error
        ? err.message
        : typeof err === "string"
          ? err
          : "Failed to send message. Is the backend running?";
    if (!isAbortError(msg)) {
      markError(msg);
    }
    finalizeSessionRun(setSessions, sessionId, !isAbortError(msg));
    setSuppressPersist(false);
  }
}

export async function abort(sessionId: string): Promise<void> {
  await abortSessionStream(setSessions, sessionId);
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

// Load persisted sessions once at module startup (before first render).
initChatPersistence();

// Register checker to avoid cyclic import in useSessionStore
registerSessionHasMessagesChecker((sessionId) => {
  return getChatSession(sessionId).messages.length > 0;
});
