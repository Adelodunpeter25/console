import { create } from "zustand";
import { toast } from "sonner";
import type { AgentMessage, AgentSessionEvent } from "@console/types";
import { tauriApi } from "../lib/tauri-api";
import { useProviderStore } from "./useProviderStore";

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

  loadSession: (sessionId: string) => Promise<void>;
  setInput: (val: string) => void;
  /**
   * Change the model for the active session. Resolves the provider from the
   * catalog, updates local state, and persists the change to the backend.
   */
  changeModel: (sessionId: string, projectId: string, modelId: string) => void;
  sendMessage: (sessionId: string) => Promise<void>;
  abort: (sessionId: string) => Promise<void>;
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

export const useChatStore = create<ChatState>((set, get) => ({
  messages: [],
  input: "",
  running: false,
  streamingText: "",
  streamingThinking: "",
  sessionModelId: null,
  sessionProvider: null,

  loadSession: async (sessionId: string) => {
    try {
      const detail = await tauriApi.getSession(sessionId);
      set({
        messages: detail.messages,
        streamingText: "",
        streamingThinking: "",
        sessionModelId: detail.header.modelId ?? null,
        sessionProvider: detail.header.provider ?? null,
      });
    } catch {
      set({
        messages: [],
        streamingText: "",
        streamingThinking: "",
        sessionModelId: null,
        sessionProvider: null,
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

  sendMessage: async (sessionId: string) => {
    const { input, running, sessionModelId, sessionProvider } = get();
    const prompt = input.trim();
    if (!prompt || running) return;

    set((s) => ({
      input: "",
      running: true,
      messages: [...s.messages, { role: "user", content: prompt }],
      streamingText: "",
      streamingThinking: "",
    }));

    let unlisten: (() => void) | null = null;
    let hadError = false;

    try {
      unlisten = await tauriApi.listenAgentEvents(sessionId, (event) => {
        get().handleEvent(event);
      });
      await tauriApi.runAgent(
        sessionId,
        prompt,
        sessionModelId ?? undefined,
        sessionProvider ?? undefined,
      );
    } catch (err) {
      hadError = true;
      const msg =
        err instanceof Error ? err.message : "Failed to send message. Is the backend running?";
      toast.error(msg);
      // Show the error inline as an assistant message so the user sees it.
      set((s) => ({
        messages: [
          ...s.messages,
          {
            role: "assistant",
            content: [{ type: "text", text: `Error: ${msg}` }],
          },
        ],
      }));
    } finally {
      if (unlisten) unlisten();
      set({ running: false, streamingText: "", streamingThinking: "" });
      // Only reload from server if the stream completed successfully —
      // reloading after an error would wipe the error message we just added.
      if (!hadError) {
        await get().loadSession(sessionId);
      }
    }
  },

  abort: async (sessionId: string) => {
    try {
      await tauriApi.abortRun(sessionId);
    } catch {
      // ignore
    }
    set({ running: false, streamingText: "", streamingThinking: "" });
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
    }),

  handleEvent: (event: AgentSessionEvent) => {
    switch (event.type) {
      case "modelStreamPart":
        if (event.part.text) {
          set((s) => ({ streamingText: s.streamingText + event.part.text! }));
        }
        if (event.part.thinking) {
          set((s) => ({ streamingThinking: s.streamingThinking + event.part.thinking! }));
        }
        break;
      case "modelStreamEnd":
        if (event.turn?.content) {
          set((s) => ({
            messages: [...s.messages, event.turn],
            streamingText: "",
            streamingThinking: "",
          }));
        }
        break;
      case "toolExecutionEnd":
        set((s) => ({
          messages: [...s.messages, { role: "toolResult", results: event.results }],
        }));
        break;
      case "error":
        set((s) => ({
          messages: [
            ...s.messages,
            {
              role: "assistant",
              content: [
                {
                  type: "text",
                  text: `Error: ${event.error.message}`,
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
