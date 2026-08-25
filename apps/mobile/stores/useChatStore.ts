import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { AgentMessage, AgentSessionEvent, ImageAttachment } from "@console/types";
import type { ChatSessionState, ChatSnapshot } from "@/types";
import { createChatSessionState, EMPTY_CHAT_SESSION } from "@/types/chat-state";
import { applyChatEvent, toChatSnapshot } from "@/utils/chat-events";
import { reconstructRuns } from "@/utils/reconstruct-runs";
import { startNativeChatStream } from "@/utils/native-stream";
import { useSessionStore, registerSessionHasMessagesChecker } from "./useSessionStore";
import { useProviderStore } from "./useProviderStore";
import { chatPersistConfig, setSuppressPersist } from "./chat/chat-persist";
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
import { app$ } from "./useAppStore";

export interface ChatStoreState {
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

// --- Streaming coalescing ---
const _streamBuf: Record<string, { text: string; thinking: string }> = {};
const _streamRaf: Record<string, ReturnType<typeof requestAnimationFrame>> = {};

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
              // Keep draft input/attachments across reloads so image drafts
              // survive going back to home and re-entering the chat.
              // sendMessage and explicit clear/attachment actions manage them.
            };
          }),
        }));
      },

      setInput: (sessionId, input) =>
        set((state) => ({
          sessions: updateSession(state.sessions, sessionId, (current) => ({
            ...current,
            input,
            draftUpdatedAt: input.trim().length > 0 || current.attachments.length > 0 ? Date.now() : undefined,
          })),
        })),

      addAttachments: (sessionId, attachments) => {
        if (attachments.length === 0) return;
        set((state) => ({
          sessions: updateSession(state.sessions, sessionId, (current) => {
            const merged = trimDraftAttachments([...current.attachments, ...attachments]);
            return {
              ...current,
              attachments: merged,
              draftUpdatedAt: Date.now(),
            };
          }),
        }));
      },

      removeAttachment: (sessionId, index) =>
        set((state) => ({
          sessions: updateSession(state.sessions, sessionId, (current) => {
            const next = current.attachments.filter((_, i) => i !== index);
            return {
              ...current,
              attachments: next,
              draftUpdatedAt: current.input.trim().length > 0 || next.length > 0 ? Date.now() : undefined,
            };
          }),
        })),

      clearAttachments: (sessionId) =>
        set((state) => ({
          sessions: updateSession(state.sessions, sessionId, (current) => ({
            ...current,
            attachments: [],
            draftUpdatedAt: current.input.trim().length > 0 ? Date.now() : undefined,
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
                get().handleEvent(sessionId, event);
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

      handleEvent: (sessionId, event) => {
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
            set((state) => ({
              sessions: updateSession(state.sessions, sessionId, (s) => ({
                ...s,
                streamingText: s.streamingText + pending.text,
                streamingThinking: s.streamingThinking + pending.thinking,
              })),
            }));
          });
          return;
        }
        // All other events flush any pending text buffer first, then apply immediately
        if (_streamBuf[sessionId]) {
          if (_streamRaf[sessionId] != null) cancelAnimationFrame(_streamRaf[sessionId]!);
          const pending = _streamBuf[sessionId]!;
          delete _streamBuf[sessionId];
          delete _streamRaf[sessionId];
          set((state) => ({
            sessions: updateSession(state.sessions, sessionId, (s) => ({
              ...s,
              streamingText: s.streamingText + pending.text,
              streamingThinking: s.streamingThinking + pending.thinking,
            })),
          }));
        }
        set((state) => ({
          sessions: updateSession(state.sessions, sessionId, (sessionState) =>
            applyChatEvent(sessionState, event),
          ),
        }));
      },

      getSession: (sessionId) => get().sessions[sessionId] ?? EMPTY_CHAT_SESSION,

      getSnapshot: (sessionId) => toChatSnapshot(get().getSession(sessionId)),
    }),
    chatPersistConfig,
  ),
);

// Register checker to avoid cyclic import in useSessionStore
registerSessionHasMessagesChecker((sessionId) => {
  return useChatStore.getState().getSession(sessionId).messages.length > 0;
});
