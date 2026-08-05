import { create } from "zustand";
import { toast } from "sonner";
import type { ImageAttachment } from "@console/types";
import { tauriApi } from "../lib/tauri-api";
import { useProviderStore } from "./useProviderStore";
import { useProjectStore } from "./useProjectStore";
import { useSessionStore } from "./useSessionStore";
import { useSessionStatusStore } from "./useSessionStatusStore";
import type { ChatStoreState } from "../types/chat";
import {
  createChatSessionState,
  getChatSessionState,
  updateChatSession,
} from "../types/chat-state";
import { applyChatEvent } from "./chat-events";
import { reconstructRuns } from "../utils/useMessageHistory.js";

type ChatState = ChatStoreState;

export { EMPTY_CHAT_SESSION } from "../types/chat-state";

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

/** Push a status update to the project store so the sidebar reflects it. */
function syncSessionStatus(
  sessionId: string,
  status: "idle" | "working" | "done" | "needs_attention",
) {
  useSessionStatusStore.getState().setStatus(sessionId, status);
}

export const useChatStore = create<ChatState>((set, get) => ({
  sessions: {},

  loadMessages: (sessionId, messages) => {
    set((state) => ({
      sessions: updateChatSession(state.sessions, sessionId, (current) => {
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
          pendingQuestion: null,
          pendingPermissions: [],
          runs: reconstructRuns(messages),
          attachments: [],
        };
      }),
    }));
  },

  setInput: (sessionId, input) =>
    set((state) => ({
      sessions: updateChatSession(state.sessions, sessionId, (current) => ({ ...current, input })),
    })),

  pickImages: async (sessionId) => {
    try {
      const picked = await tauriApi.pickImages();
      if (picked.length > 0) {
        const attachments: ImageAttachment[] = picked.map((p) => ({
          data: p.data,
          mimeType: p.mimeType,
        }));
        set((state) => ({
          sessions: updateChatSession(state.sessions, sessionId, (current) => ({
            ...current,
            attachments: [...current.attachments, ...attachments],
          })),
        }));
      }
    } catch (err) {
      toast.error("Failed to pick images.");
      console.error("pickImages error:", err);
    }
  },

  removeAttachment: (sessionId, index) =>
    set((state) => ({
      sessions: updateChatSession(state.sessions, sessionId, (current) => ({
        ...current,
        attachments: current.attachments.filter((_, i) => i !== index),
      })),
    })),

  addAttachments: (sessionId, attachments) => {
    if (attachments.length === 0) return;
    set((state) => ({
      sessions: updateChatSession(state.sessions, sessionId, (current) => ({
        ...current,
        attachments: [...current.attachments, ...attachments],
      })),
    }));
  },

  sendMessage: async (sessionId: string) => {
    const current = getChatSessionState(get().sessions, sessionId);
    const { input, running, attachments } = current;
    const { sessionModelId, sessionProvider, approvalMode } = useSessionStore.getState();
    const prompt = input.trim();
    if (!prompt || running) return;

    if (attachments.length > 0 && sessionModelId && sessionProvider) {
      const selectedModel = useProviderStore
        .getState()
        .modelsByProvider[sessionProvider]?.find((model) => model.id === sessionModelId);
      if (selectedModel?.supportsImages === false) {
        toast.error(`The selected model '${sessionModelId}' does not support image attachments.`);
        return;
      }
    }

    set((state) => ({
      sessions: updateChatSession(state.sessions, sessionId, (session) => ({
        ...session,
        input: "",
        running: true,
        messages: [
          ...session.messages,
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
          ...session.runs,
          {
            runId: crypto.randomUUID(),
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

    let unlisten: (() => void) | null = null;
    let hadError = false;
    const markError = (msg: string) => {
      hadError = true;
      toast.error(msg);
      set((state) => ({
        sessions: updateChatSession(state.sessions, sessionId, (session) => ({
          ...session,
          messages: [
            ...session.messages,
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
      // Subscribe before invoking so early SSE frames aren't dropped.
      unlisten = await tauriApi.listenAgentEvents(sessionId, (event) => {
        if (event.type === "error") {
          if (isAbortError(event.error.message)) {
            hadError = true; // prevent reload, but don't toast or show inline error
          } else {
            hadError = true;
            toast.error(event.error.message);
          }
        }
        // Sync needs_attention to the sidebar when the agent pauses.
        if (event.type === "askQuestion" || event.type === "permissionRequest") {
          syncSessionStatus(sessionId, "needs_attention");
        }
        get().handleEvent(sessionId, event);
      });
      await tauriApi.runAgent(
        sessionId,
        prompt,
        sessionModelId ?? undefined,
        sessionProvider ?? undefined,
        approvalMode,
        attachments,
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
    } finally {
      if (unlisten) {
        const fn = unlisten;
        setTimeout(() => {
          try {
            fn();
          } catch (err) {
            console.error("Failed to unlisten agent events:", err);
          }
        }, 100);
      }
      set((state) => ({
        sessions: updateChatSession(state.sessions, sessionId, (session) => {
          // Finalize the latest run: compute elapsed time and mark as
          // completed if still working (sessionEnd may not have fired).
          const runs = session.runs.length > 0 ? [...session.runs] : [];
          if (runs.length > 0) {
            const latest = runs[runs.length - 1]!;
            if (latest.status === "working") {
              runs[runs.length - 1] = {
                ...latest,
                status: hadError ? "failed" : "completed",
                elapsedMs: latest.startedAt
                  ? Date.now() - latest.startedAt
                  : latest.elapsedMs,
              };
            }
          }
          return {
            ...session,
            running: false,
            streamingText: "",
            streamingThinking: "",
            activeToolCalls: [],
            runs,
          };
        }),
      }));
      // Sync sidebar status based on whether the run succeeded or had an error.
      // Do NOT reload the session here — it replaces the entire messages array with
      // only DB-persisted data, which permanently wipes any in-memory error bubbles
      // that appeared before this run. All messages are already correct in memory
      // via handleEvent (modelStreamEnd, toolExecutionEnd, etc.).
      syncSessionStatus(sessionId, hadError ? "needs_attention" : "done");
      // Refresh the header so a first-prompt auto-rename of the title shows
      // up in the sidebar (the server renames it in the DB when the run starts).
      useProjectStore.getState().refreshSessionHeader(sessionId);
    }
  },

  abort: async (sessionId: string) => {
    try {
      await tauriApi.abortRun(sessionId);
    } catch {
      // ignore
    }
    set((state) => ({
      sessions: updateChatSession(state.sessions, sessionId, (session) => {
        const runs = session.runs.length > 0 ? [...session.runs] : [];
        if (runs.length > 0) {
          const latest = runs[runs.length - 1]!;
          if (latest.status === "working") {
            runs[runs.length - 1] = {
              ...latest,
              status: "aborted",
              elapsedMs: latest.startedAt
                ? Date.now() - latest.startedAt
                : latest.elapsedMs,
            };
          }
        }
        return {
          ...session,
          running: false,
          streamingText: "",
          streamingThinking: "",
          pendingQuestion: null,
          pendingPermissions: [],
          activeToolCalls: [],
          runs,
        };
      }),
    }));
    syncSessionStatus(sessionId, "done");
  },

  answerQuestion: async (sessionId: string, requestId: string, answer: string | string[]) => {
    try {
      await tauriApi.answerQuestion(sessionId, requestId, answer);
    } catch (err) {
      toast.error("Failed to send answer. Please try again.");
      console.error("answerQuestion error:", err);
    } finally {
      // The server consumes each pending request exactly once. Whether the
      // answer was delivered or not, clear it from the UI so the panel can't
      // get stuck on an already-consumed (or failed) request.
      set((state) => ({
        sessions: updateChatSession(state.sessions, sessionId, (session) => ({
          ...session,
          pendingQuestion:
            session.pendingQuestion?.request.requestId === requestId
              ? null
              : session.pendingQuestion,
        })),
      }));
      syncSessionStatus(
        sessionId,
        getChatSessionState(get().sessions, sessionId).pendingQuestion
          ? "needs_attention"
          : "working",
      );
    }
  },

  approvePermission: async (sessionId: string, requestId: string, allow: boolean) => {
    try {
      await tauriApi.approvePermission(sessionId, requestId, allow);
    } catch (err) {
      toast.error("Failed to send approval. Please try again.");
      console.error("approvePermission error:", err);
    } finally {
      // The server consumes each pending request exactly once. Whether the
      // decision was delivered or not, remove it from the UI so a failed or
      // already-consumed request can't wedge the permission panel.
      set((state) => ({
        sessions: updateChatSession(state.sessions, sessionId, (session) => ({
          ...session,
          pendingPermissions: session.pendingPermissions.filter(
            (p) => p.request.requestId !== requestId,
          ),
        })),
      }));
      syncSessionStatus(
        sessionId,
        getChatSessionState(get().sessions, sessionId).pendingPermissions.length > 0
          ? "needs_attention"
          : "working",
      );
    }
  },

  clear: (sessionId) =>
    set((state) => ({
      sessions: updateChatSession(state.sessions, sessionId, () => createChatSessionState()),
    })),

  handleEvent: (sessionId, event) =>
    set((state) => ({
      sessions: updateChatSession(state.sessions, sessionId, (session) =>
        applyChatEvent(session, event),
      ),
    })),
}));
