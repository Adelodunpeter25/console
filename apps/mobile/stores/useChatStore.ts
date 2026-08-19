import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { AgentMessage, AgentSessionEvent, ImageAttachment } from "@console/types";
import { runService } from "@console/api";
import type { ChatSessionState, ChatSnapshot } from "../types";
import { createChatSessionState, EMPTY_CHAT_SESSION } from "../types/chat-state";
import { createSseParser } from "../utils/sse";
import { startNativeChatStream } from "../utils/native-stream";
import { applyChatEvent, toChatSnapshot } from "../utils/chat-events";
import { reconstructRuns } from "../utils/reconstruct-runs";
import { debouncedAsyncStorage } from "../utils/debounced-storage";
import { useAppStore } from "./useAppStore";
import { useSessionStore } from "./useSessionStore";
import { useProviderStore } from "./useProviderStore";
import { useSessionStatusStore } from "./useSessionStatusStore";
import { useProjectStore } from "./useProjectStore";

/** Abort-related error messages that should not be surfaced to the user. */
const ABORT_MESSAGES = [
  "This operation was aborted.",
  "Run was aborted.",
  "The operation was aborted.",
  "aborted",
];

function isAbortError(msg: string): boolean {
  const lower = msg.toLowerCase();
  return ABORT_MESSAGES.some((m) => lower.includes(m.toLowerCase()));
}

/** Push a status update to the session status store (sidebar/list dot). */
function syncSessionStatus(
  sessionId: string,
  status: "idle" | "working" | "done" | "needs_attention",
) {
  useSessionStatusStore.getState().setStatus(sessionId, status);
}

interface ChatStoreState {
  sessions: Record<string, ChatSessionState>;
  loadMessages: (sessionId: string, messages: AgentMessage[]) => void;
  setInput: (sessionId: string, value: string) => void;
  addAttachments: (sessionId: string, attachments: ImageAttachment[]) => void;
  removeAttachment: (sessionId: string, index: number) => void;
  clearAttachments: (sessionId: string) => void;
  sendMessage: (sessionId: string, prompt?: string) => Promise<void>;
  abort: (sessionId: string) => Promise<void>;
  answerQuestion: (
    sessionId: string,
    requestId: string,
    answer: string | string[],
  ) => Promise<void>;
  approvePermission: (sessionId: string, requestId: string, allow: boolean) => Promise<void>;
  clear: (sessionId: string) => void;
  reset: (sessionId: string) => void;
  handleEvent: (sessionId: string, event: AgentSessionEvent) => void;
  /** Get a session's raw runtime state. */
  getSession: (sessionId: string) => ChatSessionState;
  /** Derive the UI snapshot for a session. */
  getSnapshot: (sessionId: string) => ChatSnapshot;
}

function updateSession(
  sessions: Record<string, ChatSessionState>,
  sessionId: string,
  update: (state: ChatSessionState) => ChatSessionState,
): Record<string, ChatSessionState> {
  return {
    ...sessions,
    [sessionId]: update(sessions[sessionId] ?? EMPTY_CHAT_SESSION),
  };
}

/* ------------------------------------------------------------------ */
/* Local persistence (cold-start cache)                                */
/* ------------------------------------------------------------------ */
//
// The chat store is persisted to AsyncStorage so that after an app restart
// the last-seen messages for each session render instantly — before the
// server has responded. Only data fields are persisted (never the action
// functions or transient streaming state), and the persisted blob is capped
// to avoid overflowing AsyncStorage's per-key size limit on Android.
//
// Writes are debounced (2s) via the debouncedAsyncStorage util so the rapid
// state changes during SSE streaming don't trigger a JSON.stringify + native
// bridge call on every token.

const PERSIST_NAME = "console-chat-cache";
const PERSIST_DEBOUNCE_MS = 2000;
const MAX_PERSISTED_SESSIONS = 25;
const MAX_PERSISTED_MESSAGES = 50;

export const useChatStore = create<ChatStoreState>()(
  persist(
    (set, get) => ({
  sessions: {},

  loadMessages: (sessionId, messages) => {
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, (current) => {
        // Never replace an active run with a stale reload while navigating
        // between sessions. The server remains the source of truth once the
        // run has settled.
        if (current.running) return current;
        return {
          ...current,
          messages,
          streamingText: "",
          streamingThinking: "",
          activeToolCalls: [],
          pendingQuestions: [],
          pendingPermissions: [],
          runs: reconstructRuns(messages),
          attachments: [],
        };
      }),
    }));
  },

  setInput: (sessionId, input) =>
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, (current) => ({ ...current, input })),
    })),

  addAttachments: (sessionId, attachments) => {
    if (attachments.length === 0) return;
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, (current) => ({
        ...current,
        attachments: [...current.attachments, ...attachments],
      })),
    }));
  },

  removeAttachment: (sessionId, index) =>
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, (current) => ({
        ...current,
        attachments: current.attachments.filter((_, i) => i !== index),
      })),
    })),

  clearAttachments: (sessionId) =>
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, (current) => ({
        ...current,
        attachments: [],
      })),
    })),

  sendMessage: async (sessionId, promptOverride) => {
    const session = get().getSession(sessionId);
    const { input, running, attachments } = session;
    const prompt = (promptOverride ?? input).trim();
    if (!prompt || running) return;

    const { sessionModelId, sessionProvider, approvalMode } = useSessionStore
      .getState()
      .getSession(sessionId);

    // Validate image support for the selected model.
    if (attachments.length > 0 && sessionModelId && sessionProvider) {
      const selectedModel = useProviderStore
        .getState()
        .modelsByProvider[sessionProvider]?.find((model) => model.id === sessionModelId);
      if (selectedModel?.supportsImages === false) {
        // Surface as an inline error message.
        set((state) => ({
          sessions: updateSession(state.sessions, sessionId, (current) => ({
            ...current,
            messages: [
              ...current.messages,
              {
                role: "assistant",
                content: [
                  {
                    type: "text",
                    text: `Error: The selected model '${sessionModelId}' does not support image attachments.`,
                  },
                ],
              },
            ],
          })),
        }));
        return;
      }
    }

    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, (sessionState) => ({
        ...sessionState,
        input: "",
        running: true,
        messages: [
          ...sessionState.messages,
          {
            role: "user",
            content: prompt,
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
    }));
    syncSessionStatus(sessionId, "working");

    const markError = (msg: string) => {
      set((state) => ({
        sessions: updateSession(state.sessions, sessionId, (sessionState) => ({
          ...sessionState,
          messages: [
            ...sessionState.messages,
            {
              role: "assistant",
              content: [{ type: "text", text: `Error: ${msg}` }],
            },
          ],
          streamingText: "",
          streamingThinking: "",
        })),
      }));
    };

    try {
      const baseUrl = useAppStore.getState().backendUrl ?? "";
      let hadError = false;

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
            get().handleEvent(sessionId, event);
          },
          onError: (errMsg) => {
            if (!isAbortError(errMsg)) {
              hadError = true;
              markError(errMsg);
            }
          },
          onEnd: (aborted) => {
            finalizeRun(sessionId, hadError && !aborted);
          },
        }
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
      finalizeRun(sessionId, !isAbortError(msg));
    }
  },

  abort: async (sessionId: string) => {
    try {
      await runService.abortRun(sessionId);
    } catch {
      // ignore
    }
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, (sessionState) => {
        const runs = sessionState.runs.length > 0 ? [...sessionState.runs] : [];
        if (runs.length > 0) {
          const latest = runs[runs.length - 1]!;
          if (latest.status === "working") {
            runs[runs.length - 1] = {
              ...latest,
              status: "aborted",
              elapsedMs: latest.startedAt ? Date.now() - latest.startedAt : latest.elapsedMs,
            };
          }
        }
        return {
          ...sessionState,
          running: false,
          streamingText: "",
          streamingThinking: "",
          pendingQuestions: [],
          pendingPermissions: [],
          activeToolCalls: [],
          runs,
        };
      }),
    }));
    syncSessionStatus(sessionId, "done");
  },

  answerQuestion: async (sessionId, requestId, answer) => {
    try {
      await runService.answerQuestion(sessionId, { requestId, answer });
    } catch (err) {
      console.error("answerQuestion error:", err);
    } finally {
      // The server consumes each pending request exactly once. Whether the
      // answer was delivered or not, remove it from the queue so the panel
      // can't get stuck on an already-consumed (or failed) request.
      set((state) => ({
        sessions: updateSession(state.sessions, sessionId, (sessionState) => ({
          ...sessionState,
          pendingQuestions: sessionState.pendingQuestions.filter(
            (q) => q.request.requestId !== requestId,
          ),
        })),
      }));
      const pendingCount = get().sessions[sessionId]?.pendingQuestions.length ?? 0;
      syncSessionStatus(sessionId, pendingCount > 0 ? "needs_attention" : "working");
    }
  },

  approvePermission: async (sessionId, requestId, allow) => {
    try {
      await runService.approvePermission(sessionId, { requestId, allow });
    } catch (err) {
      console.error("approvePermission error:", err);
    } finally {
      set((state) => ({
        sessions: updateSession(state.sessions, sessionId, (sessionState) => ({
          ...sessionState,
          pendingPermissions: sessionState.pendingPermissions.filter(
            (p) => p.request.requestId !== requestId,
          ),
        })),
      }));
      const pendingCount = get().sessions[sessionId]?.pendingPermissions.length ?? 0;
      syncSessionStatus(sessionId, pendingCount > 0 ? "needs_attention" : "working");
    }
  },

  clear: (sessionId) =>
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, () => createChatSessionState()),
    })),

  reset: (sessionId) =>
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, () => createChatSessionState()),
    })),

  handleEvent: (sessionId, event) =>
    set((state) => ({
      sessions: updateSession(state.sessions, sessionId, (sessionState) =>
        applyChatEvent(sessionState, event),
      ),
    })),

  getSession: (sessionId) => get().sessions[sessionId] ?? EMPTY_CHAT_SESSION,

  getSnapshot: (sessionId) => toChatSnapshot(get().getSession(sessionId)),
    }),
    {
      name: PERSIST_NAME,
      storage: createJSONStorage(() => debouncedAsyncStorage(PERSIST_DEBOUNCE_MS)),
      // Only persist the data that matters for cold-start: messages, runs
      // (the run timeline), and the current input draft. Transient fields
      // (running, streaming buffers, pending queues, attachments) are
      // stripped — they're meaningless after a restart.
      partialize: (state) => ({
        sessions: Object.fromEntries(
          Object.entries(state.sessions)
            .filter(([, s]) => s.messages.length > 0)
            // Keep the sessions with the most messages (rough proxy for
            // "most recently active") and cap to stay under AsyncStorage's
            // per-key size limit on Android.
            .sort((a, b) => b[1].messages.length - a[1].messages.length)
            .slice(0, MAX_PERSISTED_SESSIONS)
            .map(([id, s]) => [
              id,
              {
                messages: s.messages.slice(-MAX_PERSISTED_MESSAGES),
                runs: s.runs,
                input: s.input,
              },
            ]),
        ),
      }),
      // Merge rehydrated partial sessions over the full default state so
      // every session has all ChatSessionState fields (running, streaming,
      // etc.) — not just the three we persisted.
      merge: (persisted, current) => {
        const p = persisted as { sessions?: Record<string, Partial<ChatSessionState>> };
        if (!p?.sessions) return current as ChatStoreState;
        const sessions: Record<string, ChatSessionState> = {};
        for (const [id, partial] of Object.entries(p.sessions)) {
          sessions[id] = { ...createChatSessionState(), ...partial };
        }
        return { ...(current as ChatStoreState), sessions };
      },
    },
  ),
);

/** Finalize the latest run after the stream settles. */
function finalizeRun(sessionId: string, hadError: boolean): void {
  useChatStore.setState((state) => ({
    sessions: updateSession(state.sessions, sessionId, (sessionState) => {
      const runs = sessionState.runs.length > 0 ? [...sessionState.runs] : [];
      if (runs.length > 0) {
        const latest = runs[runs.length - 1]!;
        if (latest.status === "working") {
          runs[runs.length - 1] = {
            ...latest,
            status: hadError ? "failed" : "completed",
            elapsedMs: latest.startedAt ? Date.now() - latest.startedAt : latest.elapsedMs,
          };
        }
      }
      return {
        ...sessionState,
        running: false,
        streamingText: "",
        streamingThinking: "",
        activeToolCalls: [],
        runs,
      };
    }),
  }));
  syncSessionStatus(sessionId, hadError ? "needs_attention" : "done");
  // Refresh the header so a first-prompt auto-rename of the title shows up
  // in the sidebar/list (the server renames it in the DB when the run starts).
  useProjectStore.getState().refreshSessionHeader(sessionId).catch(() => {});
}

function randomUUID(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `run-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}
