import { create } from "zustand";
import { toast } from "sonner";
import type { AgentMessage, AgentSessionEvent } from "@console/types";
import { tauriApi } from "../lib/tauri-api";

interface ChatState {
  messages: AgentMessage[];
  input: string;
  running: boolean;
  streamingText: string;
  streamingThinking: string;
  loadSession: (sessionId: string) => Promise<void>;
  setInput: (val: string) => void;
  sendMessage: (sessionId: string) => Promise<void>;
  abort: (sessionId: string) => Promise<void>;
  clear: () => void;
  handleEvent: (event: AgentSessionEvent) => void;
}

export const useChatStore = create<ChatState>((set, get) => ({
  messages: [],
  input: "",
  running: false,
  streamingText: "",
  streamingThinking: "",

  loadSession: async (sessionId: string) => {
    try {
      const detail = await tauriApi.getSession(sessionId);
      set({ messages: detail.messages, streamingText: "", streamingThinking: "" });
    } catch {
      set({ messages: [], streamingText: "", streamingThinking: "" });
    }
  },

  setInput: (input) => set({ input }),

  sendMessage: async (sessionId: string) => {
    const prompt = get().input.trim();
    if (!prompt || get().running) return;

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
      await tauriApi.runAgent(sessionId, prompt);
    } catch (err) {
      hadError = true;
      const msg = err instanceof Error ? err.message : "Failed to send message. Is the backend running?";
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
    set({ messages: [], input: "", running: false, streamingText: "", streamingThinking: "" }),

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
