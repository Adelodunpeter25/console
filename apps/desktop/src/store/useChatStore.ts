import { create } from "zustand";
import type { AgentMessage, AgentSessionEvent } from "@console/types";
import { tauriApi } from "../lib/tauri-api";

interface ChatState {
  messages: AgentMessage[];
  input: string;
  running: boolean;
  streamingText: string;
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

  loadSession: async (sessionId: string) => {
    try {
      const detail = await tauriApi.getSession(sessionId);
      set({ messages: detail.messages, streamingText: "" });
    } catch {
      set({ messages: [], streamingText: "" });
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
    }));

    let unlisten: (() => void) | null = null;
    try {
      unlisten = await tauriApi.listenAgentEvents((event) => {
        get().handleEvent(event);
      });
      await tauriApi.runAgent(sessionId, prompt);
    } catch {
      // On error, reload the session to get the true state
      await get().loadSession(sessionId);
    } finally {
      if (unlisten) unlisten();
      set({ running: false, streamingText: "" });
      await get().loadSession(sessionId);
    }
  },

  abort: async (sessionId: string) => {
    try {
      await tauriApi.abortRun(sessionId);
    } catch {
      // ignore
    }
    set({ running: false, streamingText: "" });
  },

  clear: () => set({ messages: [], input: "", running: false, streamingText: "" }),

  handleEvent: (event: AgentSessionEvent) => {
    switch (event.type) {
      case "modelStreamPart":
        if (event.part.text) {
          set((s) => ({ streamingText: s.streamingText + event.part.text! }));
        }
        break;
      case "modelStreamEnd":
        if (event.turn?.content) {
          set((s) => ({
            messages: [...s.messages, event.turn],
            streamingText: "",
          }));
        }
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
        }));
        break;
      default:
        break;
    }
  },
}));
