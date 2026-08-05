import { create } from "zustand";
import type { ChatSnapshot } from "../types";
import { emptyChatSnapshot } from "../types";

interface ChatState extends ChatSnapshot {
  /** Replace the whole snapshot (used by the stream reducer commit). */
  setSnapshot: (snapshot: ChatSnapshot) => void;
  /** Clear pending permission + question, e.g. after answering. */
  clearPending: () => void;
  /** Reset to a fresh snapshot when switching sessions. */
  reset: () => void;
}

export const useChatStore = create<ChatState>((set) => ({
  ...emptyChatSnapshot,

  setSnapshot: (snapshot) => set(snapshot),

  clearPending: () =>
    set((state) => ({
      ...state,
      pendingPermission: null,
      pendingQuestion: null,
    })),

  reset: () => set(emptyChatSnapshot),
}));
