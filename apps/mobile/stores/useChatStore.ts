import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { AgentMessage, AgentSessionEvent, ImageAttachment } from "@console/types";
import type { ChatSessionState, ChatSnapshot } from "../types";
import { createChatSessionState, EMPTY_CHAT_SESSION } from "../types/chat-state";
import { applyChatEvent, toChatSnapshot } from "../utils/chat-events";
import { reconstructRuns } from "../utils/reconstruct-runs";
import { startNativeChatStream } from "../utils/native-stream";
import { useAppStore } from "./useAppStore";
import { useSessionStore } from "./useSessionStore";
import { useProviderStore } from "./useProviderStore";
import { chatPersistConfig } from "./chat/chat-persist";
import {
  updateSession,
  syncSessionStatus,
  randomUUID,
  isAbortError,
  finalizeSessionRun,
  abortSessionStream,
} from "./chat/chat-stream-runner";
import { answerSessionQuestion, approveSessionPermission } from "./chat/chat-decisions";

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
  getSession: (sessionId: string) => ChatSessionState;
  getSnapshot: (sessionId: string) => ChatSnapshot;
}

export const useChatStore = create<ChatStoreState>()(
  persist(
    (set, get) => ({
      sessions: {},

      loadMessages: (sessionId, messages) => {
        set((state) => ({
          sessions: updateSession(state.sessions, sessionId, (current) => {
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
            set((state) => ({
              sessions: updateSession(state.sessions, sessionId, (current) => ({
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
                  createdAt: Date.now(),
                  content: [{ type: "text", text: `Error: ${msg}` }],
                },
              ],
              streamingText: "",
              streamingThinking: "",
            })),
          }));
        };

        const setSessions = (
          updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>,
        ) => set((state) => ({ sessions: updater(state.sessions) }));

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
                finalizeSessionRun(setSessions, sessionId, hadError && !aborted);
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
        }
      },

      abort: async (sessionId: string) => {
        const setSessions = (
          updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>,
        ) => set((state) => ({ sessions: updater(state.sessions) }));
        await abortSessionStream(setSessions, sessionId);
      },

      answerQuestion: async (sessionId, requestId, answer) => {
        const setSessions = (
          updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>,
        ) => set((state) => ({ sessions: updater(state.sessions) }));
        await answerSessionQuestion(setSessions, () => get().sessions, sessionId, requestId, answer);
      },

      approvePermission: async (sessionId, requestId, allow) => {
        const setSessions = (
          updater: (sessions: Record<string, ChatSessionState>) => Record<string, ChatSessionState>,
        ) => set((state) => ({ sessions: updater(state.sessions) }));
        await approveSessionPermission(setSessions, () => get().sessions, sessionId, requestId, allow);
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
    chatPersistConfig,
  ),
);
