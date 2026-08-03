import { create } from "zustand";
import { toast } from "sonner";
import type { AgentMessage, AgentSessionEvent, AskQuestionRequest, PermissionRequest, ApprovalMode, ToolResult, ProjectInfo } from "@console/types";
import { tauriApi } from "../lib/tauri-api";
import { useProviderStore } from "./useProviderStore";
import { useProjectStore } from "./useProjectStore";
import { useAppStore } from "./useAppStore";

/** A pending question from the agent's ask tool, awaiting user input. */
export interface PendingQuestion {
  request: AskQuestionRequest;
}

/** A pending tool permission request, awaiting user approval. */
export interface PendingPermission {
  request: PermissionRequest;
}

interface ChatState {
  messages: AgentMessage[];
  input: string;
  running: boolean;
  streamingText: string;
  streamingThinking: string;
  /** Model ID for the active session (persisted per-session). */
  sessionModelId: string | null;
  /** Provider for the active session (persisted per-session). */
  sessionProvider: string | null;
  /** Working directory for the active session (persisted per-session). */
  sessionCwd: string | null;
  /** Approval mode for the agent (always-ask, accept-edits, plan-mode, full-access). */
  approvalMode: ApprovalMode;
  /** The session currently displayed in the chat view. SSE events from
      other sessions are dropped so they don't contaminate the active view. */
  activeSessionId: string | null;
  /** Pending ask-question request from the agent, if any. */
  pendingQuestion: PendingQuestion | null;
  /** Pending permission request from the agent, if any. */
  pendingPermission: PendingPermission | null;
  /** Tool results arriving in real-time via `toolExecutionResult` events.
      Cleared when `toolExecutionEnd` finalises the batch. */
  liveToolResults: ToolResult[];

  loadSession: (sessionId: string) => Promise<void>;
  setInput: (val: string) => void;
  /**
   * Change the model for the active session. Resolves the provider from the
   * catalog, updates local state, and persists the change to the backend.
   */
  changeModel: (sessionId: string, projectId: string, modelId: string) => void;
  /** Set the working folder for the active session from a backend project. */
  changeProject: (sessionId: string, project: ProjectInfo) => void;
  /** Set the approval mode for agent runs. Persists to backend if a session is active. */
  setApprovalMode: (mode: ApprovalMode) => void;
  sendMessage: (sessionId: string) => Promise<void>;
  abort: (sessionId: string) => Promise<void>;
  /** Answer a pending question from the agent. */
  answerQuestion: (sessionId: string, requestId: string, answer: string | string[]) => Promise<void>;
  /** Approve or deny a pending permission request. */
  approvePermission: (sessionId: string, requestId: string, allow: boolean) => Promise<void>;
  clear: () => void;
  handleEvent: (event: AgentSessionEvent) => void;
}

/**
 * Resolve the provider for a given model ID by scanning the provider catalog.
 * Falls back to the supplied default if no match is found.
 */
function resolveProvider(modelId: string, fallback: string | null): string | null {
  const { providers, modelsByProvider } = useProviderStore.getState();
  for (const provider of providers) {
    const models = modelsByProvider[provider.name] ?? [];
    if (models.some((m) => m.id === modelId)) {
      return provider.name;
    }
  }
  return fallback;
}

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
function syncSessionStatus(sessionId: string, status: "idle" | "working" | "done" | "needs_attention") {
  useProjectStore.getState().updateSessionStatus(sessionId, status);
}

export const useChatStore = create<ChatState>((set, get) => ({
  messages: [],
  input: "",
  running: false,
  streamingText: "",
  streamingThinking: "",
  sessionModelId: null,
  sessionProvider: null,
  sessionCwd: null,
  approvalMode: "always-ask",
  activeSessionId: null,
  pendingQuestion: null,
  pendingPermission: null,
  liveToolResults: [],

  loadSession: async (sessionId: string) => {
    // Mark this as the active session so SSE events from other sessions
    // are dropped instead of contaminating the view.
    set({ activeSessionId: sessionId, streamingText: "", streamingThinking: "", liveToolResults: [] });
    try {
      const detail = await tauriApi.getSession(sessionId);
      // Guard against a rapid session switch — if the user switched again
      // while this fetch was in flight, don't overwrite the new session's state.
      if (get().activeSessionId !== sessionId) return;
      set({
        messages: detail.messages,
        streamingText: "",
        streamingThinking: "",
        sessionModelId: detail.header.modelId ?? null,
        sessionProvider: detail.header.provider ?? null,
        sessionCwd: detail.header.cwd ?? null,
        // Restore the persisted approvalMode so the UI reflects what's in the DB.
        approvalMode: (detail.header.approvalMode as ApprovalMode) ?? "always-ask",
      });
      // Sync the server's authoritative status to the sidebar.
      syncSessionStatus(sessionId, detail.header.status ?? "idle");
    } catch {
      set({
        messages: [],
        streamingText: "",
        streamingThinking: "",
        sessionModelId: null,
        sessionProvider: null,
        sessionCwd: null,
      });
    }
  },

  setInput: (input) => set({ input }),

  changeModel: (sessionId, projectId, modelId) => {
    const provider = resolveProvider(modelId, get().sessionProvider);
    set({ sessionModelId: modelId, sessionProvider: provider });

    // Persist to the backend so this session remembers its model.
    tauriApi
      .updateSession(sessionId, {
        modelId,
        provider: provider as "gemini" | "antigravity" | undefined,
      })
      .catch(() => {
        // Silently ignore — local state is already updated.
      });
  },

  setApprovalMode: (mode) => {
    set({ approvalMode: mode });
    // Persist to the backend so mode survives reloads (best-effort).
    const activeSessionId = get().activeSessionId;
    if (activeSessionId) {
      tauriApi
        .updateSession(activeSessionId, { approvalMode: mode })
        .catch(() => {
          // Silently ignore — local state is already updated.
        });
    }
  },

  changeProject: (sessionId, project) => {
    set({ sessionCwd: project.path });
    useAppStore.getState().setSelectedProjectId(project.id);

    // Persist the working folder to the backend so it survives reloads.
    tauriApi
      .updateSession(sessionId, { cwd: project.path })
      .then(() => useProjectStore.getState().refreshSessionHeader(sessionId))
      .catch(() => {
        // Silently ignore — local state is already updated.
      });
  },

  sendMessage: async (sessionId: string) => {
    const { input, running, sessionModelId, sessionProvider, approvalMode } = get();
    const prompt = input.trim();
    if (!prompt || running) return;

    set((s) => ({
      input: "",
      running: true,
      messages: [...s.messages, { role: "user", content: prompt }],
      streamingText: "",
      streamingThinking: "",
      liveToolResults: [],
    }));
    syncSessionStatus(sessionId, "working");

    let unlisten: (() => void) | null = null;
    let hadError = false;
    const markError = (msg: string) => {
      hadError = true;
      toast.error(msg);
      set((s) => ({
        messages: [
          ...s.messages,
          {
            role: "assistant",
            content: [{ type: "text", text: `Error: ${msg}` }],
          },
        ],
        streamingText: "",
        streamingThinking: "",
      }));
    };

    try {
      // Subscribe before invoking so early SSE frames aren't dropped.
      unlisten = await tauriApi.listenAgentEvents(sessionId, (event) => {
        // Drop events from a session the user has already navigated away from.
        if (get().activeSessionId !== sessionId) return;
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
        get().handleEvent(event);
      });
      await tauriApi.runAgent(
        sessionId,
        prompt,
        sessionModelId ?? undefined,
        sessionProvider ?? undefined,
        approvalMode,
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
      if (unlisten) unlisten();
      // Only update running/streaming state if this session is still active.
      // If the user switched to another session, don't wipe their view.
      if (get().activeSessionId === sessionId) {
        set({ running: false, streamingText: "", streamingThinking: "" });
      }
      // Sync sidebar status based on whether the run succeeded or had an error.
      // Do NOT call loadSession here — it replaces the entire messages array with
      // only DB-persisted data, which permanently wipes any in-memory error bubbles
      // that appeared before this run. All messages are already correct in memory
      // via handleEvent (modelStreamEnd, toolExecutionEnd, etc.).
      if (get().activeSessionId === sessionId) {
        syncSessionStatus(sessionId, hadError ? "needs_attention" : "done");
      }
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
    set({ running: false, streamingText: "", streamingThinking: "" });
    syncSessionStatus(sessionId, "done");
  },

  answerQuestion: async (sessionId: string, requestId: string, answer: string | string[]) => {
    set({ pendingQuestion: null });
    syncSessionStatus(sessionId, "working");
    try {
      await tauriApi.answerQuestion(sessionId, requestId, answer);
    } catch (err) {
      toast.error("Failed to send answer. Please try again.");
      console.error("answerQuestion error:", err);
    }
  },

  approvePermission: async (sessionId: string, requestId: string, allow: boolean) => {
    set({ pendingPermission: null });
    syncSessionStatus(sessionId, "working");
    try {
      await tauriApi.approvePermission(sessionId, requestId, allow);
    } catch (err) {
      toast.error("Failed to send approval. Please try again.");
      console.error("approvePermission error:", err);
    }
  },

  clear: () =>
    set({
      messages: [],
      input: "",
      running: false,
      streamingText: "",
      streamingThinking: "",
      sessionModelId: null,
      sessionProvider: null,
      sessionCwd: null,
      approvalMode: "always-ask",
      activeSessionId: null,
      pendingQuestion: null,
      pendingPermission: null,
      liveToolResults: [],
    }),

  handleEvent: (event: AgentSessionEvent) => {
    switch (event.type) {
      case "modelStreamPart": {
        const text = event.part?.text;
        const thinking = event.part?.thinking;
        if (text || thinking) {
          set((s) => ({
            streamingText: text ? s.streamingText + text : s.streamingText,
            streamingThinking: thinking ? s.streamingThinking + thinking : s.streamingThinking,
          }));
        }
        break;
      }
      case "modelStreamEnd":
        if (event.turn) {
          set((s) => ({
            messages: [...s.messages, event.turn],
            streamingText: "",
            streamingThinking: "",
          }));
        } else {
          // Commit any buffered stream text if the turn payload is missing.
          const { streamingText, streamingThinking } = get();
          if (streamingText || streamingThinking) {
            set((s) => ({
              messages: [
                ...s.messages,
                {
                  role: "assistant",
                  content: [
                    ...(streamingThinking
                      ? [{ type: "thinking" as const, text: streamingThinking }]
                      : []),
                    ...(streamingText ? [{ type: "text" as const, text: streamingText }] : []),
                  ],
                },
              ],
              streamingText: "",
              streamingThinking: "",
            }));
          }
        }
        break;
      case "toolExecutionResult":
        set((s) => ({
          liveToolResults: [...s.liveToolResults, event.result],
        }));
        break;
      case "toolExecutionEnd":
        set((s) => ({
          messages: [...s.messages, { role: "toolResult", results: event.results }],
          liveToolResults: [],
        }));
        break;
      case "askQuestion":
        set({ pendingQuestion: { request: event.request } });
        break;
      case "permissionRequest":
        set({ pendingPermission: { request: event.request } });
        break;
      case "error":
        if (isAbortError(event.error?.message ?? "")) {
          // User-initiated abort — don't show an inline error bubble.
          set({ streamingText: "", streamingThinking: "" });
          break;
        }
        set((s) => ({
          messages: [
            ...s.messages,
            {
              role: "assistant",
              content: [
                {
                  type: "text",
                  text: `Error: ${event.error?.message ?? "Unknown agent error"}`,
                },
              ],
            },
          ],
          streamingText: "",
          streamingThinking: "",
        }));
        break;
      default:
        break;
    }
  },
}));
