import { create } from "zustand";
import { toast } from "sonner";
import type { AgentMessage, AgentSessionEvent, AskQuestionRequest, PermissionRequest, ToolResult, ImageAttachment } from "@console/types";
import { tauriApi } from "../lib/tauri-api";
import { useProviderStore } from "./useProviderStore";
import { useProjectStore } from "./useProjectStore";
import { useAppStore } from "./useAppStore";
import { useSessionStore } from "./useSessionStore";
import { useSessionStatusStore } from "./useSessionStatusStore";

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
  /** Pending ask-question request from the agent, if any. */
  pendingQuestion: PendingQuestion | null;
  /** Pending permission request from the agent, if any. */
  pendingPermissions: PendingPermission[];
  /** Tool results arriving in real-time via `toolExecutionResult` events.
      Cleared when `toolExecutionEnd` finalises the batch. */
  liveToolResults: ToolResult[];
  /** Images picked via the native dialog, awaiting send. */
  attachments: ImageAttachment[];

  loadMessages: (sessionId: string, messages: AgentMessage[]) => void;
  setInput: (val: string) => void;
  /** Open the native image picker and append the chosen images. */
  pickImages: () => Promise<void>;
  /** Remove a pending attachment by index. */
  removeAttachment: (index: number) => void;
  sendMessage: (sessionId: string) => Promise<void>;
  abort: (sessionId: string) => Promise<void>;
  /** Answer a pending question from the agent. */
  answerQuestion: (sessionId: string, requestId: string, answer: string | string[]) => Promise<void>;
  /** Approve or deny a pending permission request. */
  approvePermission: (sessionId: string, requestId: string, allow: boolean) => Promise<void>;
  clear: () => void;
  handleEvent: (event: AgentSessionEvent) => void;
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
  useSessionStatusStore.getState().setStatus(sessionId, status);
}

export const useChatStore = create<ChatState>((set, get) => ({
  messages: [],
  input: "",
  running: false,
  streamingText: "",
  streamingThinking: "",
  pendingQuestion: null,
  pendingPermissions: [],
  liveToolResults: [],
  attachments: [],

  loadMessages: (sessionId, messages) => {
    if (useAppStore.getState().selectedSessionId !== sessionId) return;
    set({
      messages,
      streamingText: "",
      streamingThinking: "",
      liveToolResults: [],
      pendingQuestion: null,
      pendingPermissions: [],
      attachments: [],
    });
  },

  setInput: (input) => set({ input }),

  pickImages: async () => {
    try {
      const picked = await tauriApi.pickImages();
      if (picked.length > 0) {
        const attachments: ImageAttachment[] = picked.map((p) => ({
          data: p.data,
          mimeType: p.mimeType,
        }));
        set((s) => ({ attachments: [...s.attachments, ...attachments] }));
      }
    } catch (err) {
      toast.error("Failed to pick images.");
      console.error("pickImages error:", err);
    }
  },

  removeAttachment: (index) =>
    set((s) => ({
      attachments: s.attachments.filter((_, i) => i !== index),
    })),

  sendMessage: async (sessionId: string) => {
    const { input, running, attachments } = get();
    const { sessionModelId, sessionProvider, approvalMode } = useSessionStore.getState();
    const prompt = input.trim();
    if (!prompt || running) return;

    if (attachments.length > 0 && sessionModelId && sessionProvider) {
      const selectedModel =
        useProviderStore.getState().modelsByProvider[sessionProvider]?.find(
          (model) => model.id === sessionModelId,
        );
      if (selectedModel?.supportsImages === false) {
        toast.error(`The selected model '${sessionModelId}' does not support image attachments.`);
        return;
      }
    }

    set((s) => ({
      input: "",
      running: true,
      messages: [
        ...s.messages,
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
      liveToolResults: [],
      attachments: [],
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
        if (useAppStore.getState().selectedSessionId !== sessionId) return;
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
      if (unlisten) unlisten();
      // Only update running/streaming state if this session is still active.
      // If the user switched to another session, don't wipe their view.
      if (useAppStore.getState().selectedSessionId === sessionId) {
        set({ running: false, streamingText: "", streamingThinking: "" });
      }
      // Sync sidebar status based on whether the run succeeded or had an error.
      // Do NOT reload the session here — it replaces the entire messages array with
      // only DB-persisted data, which permanently wipes any in-memory error bubbles
      // that appeared before this run. All messages are already correct in memory
      // via handleEvent (modelStreamEnd, toolExecutionEnd, etc.).
      if (useAppStore.getState().selectedSessionId === sessionId) {
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
    set({
      running: false,
      streamingText: "",
      streamingThinking: "",
      pendingQuestion: null,
      pendingPermissions: [],
    });
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
      set((s) => ({
        pendingQuestion: s.pendingQuestion?.request.requestId === requestId ? null : s.pendingQuestion,
      }));
      syncSessionStatus(sessionId, get().pendingQuestion ? "needs_attention" : "working");
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
      set((s) => ({
        pendingPermissions: s.pendingPermissions.filter(
          (p) => p.request.requestId !== requestId,
        ),
      }));
      syncSessionStatus(sessionId, get().pendingPermissions.length > 0 ? "needs_attention" : "working");
    }
  },

  clear: () =>
    set({
      messages: [],
      input: "",
      running: false,
      streamingText: "",
      streamingThinking: "",
      pendingQuestion: null,
      pendingPermissions: [],
      liveToolResults: [],
      attachments: [],
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
        set((s) => {
          // Some backends emit the completion event without repeating every
          // result. Preserve the results already received live so completed
          // tool rows do not regress to a spinner on the next render.
          const results = [...event.results];
          for (const live of s.liveToolResults) {
            if (!results.some((result) => result.toolCallId === live.toolCallId)) {
              results.push(live);
            }
          }
          return {
            messages: [...s.messages, { role: "toolResult", results }],
            liveToolResults: [],
          };
        });
        break;
      case "askQuestion":
        set({ pendingQuestion: { request: event.request } });
        break;
      case "permissionRequest":
        set((s) => ({
          pendingPermissions: [...s.pendingPermissions, { request: event.request }],
        }));
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
