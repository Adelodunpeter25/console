import { createJSONStorage } from "zustand/middleware";
import type { ChatSessionState } from "../../types";
import { createChatSessionState } from "../../types/chat-state";
import { debouncedAsyncStorage } from "../../utils/debounced-storage";

export const PERSIST_NAME = "console-chat-cache";
export const PERSIST_DEBOUNCE_MS = 2000;
export const MAX_PERSISTED_SESSIONS = 25;
export const MAX_PERSISTED_MESSAGES = 50;

export interface ChatStorePersistedState {
  sessions: Record<string, ChatSessionState>;
}

export const chatPersistConfig = {
  name: PERSIST_NAME,
  storage: createJSONStorage(() => debouncedAsyncStorage(PERSIST_DEBOUNCE_MS)),
  partialize: (state: ChatStorePersistedState) => ({
    sessions: Object.fromEntries(
      Object.entries(state.sessions)
        .filter(([, s]) => s.messages.length > 0)
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
  merge: (persisted: unknown, current: ChatStorePersistedState) => {
    const p = persisted as { sessions?: Record<string, Partial<ChatSessionState>> };
    if (!p?.sessions) return current;
    const sessions: Record<string, ChatSessionState> = {};
    for (const [id, partial] of Object.entries(p.sessions)) {
      sessions[id] = { ...createChatSessionState(), ...partial };
    }
    return { ...current, sessions };
  },
};
