import { createJSONStorage } from "zustand/middleware";
import type { ChatSessionState } from "../../types";
import { createChatSessionState } from "../../types/chat-state";
import { mmkvZustandStorage } from "../../utils/storage";
import { hasPersistableDraft, trimDraftAttachments } from "./draft";

export const PERSIST_NAME = "console-chat-cache";
export const MAX_PERSISTED_SESSIONS = 25;
export const MAX_PERSISTED_MESSAGES = 50;

export interface ChatStorePersistedState {
  sessions: Record<string, ChatSessionState>;
}

export const chatPersistConfig: any = {
  name: PERSIST_NAME,
  storage: createJSONStorage(() => mmkvZustandStorage),
  partialize: (state: any) => ({
    sessions: Object.fromEntries(
      Object.entries(state.sessions)
        // Keep sessions with messages OR with a draft (input/attachments) — so a
        // never-sent new chat typed but not sent still survives restart and shows
        // as DRAFT on home. Draft attachments capped to 2 via draft.ts.
        .filter(([, s]) => s.messages.length > 0 || hasPersistableDraft(s))
        .sort((a, b) => {
          const aDraft = hasPersistableDraft(a[1]) ? (a[1].draftUpdatedAt ?? 0) : 0;
          const bDraft = hasPersistableDraft(b[1]) ? (b[1].draftUpdatedAt ?? 0) : 0;
          if (aDraft !== bDraft) return bDraft - aDraft;
          return b[1].messages.length - a[1].messages.length;
        })
        .slice(0, MAX_PERSISTED_SESSIONS)
        .map(([id, s]) => [
          id,
          {
            messages: s.messages.slice(-MAX_PERSISTED_MESSAGES),
            runs: s.runs,
            input: s.input,
            attachments: trimDraftAttachments(s.attachments),
            draftUpdatedAt: s.draftUpdatedAt,
          },
        ]),
    ),
  }),
  merge: (persisted: unknown, current: ChatStorePersistedState) => {
    const p = persisted as { sessions?: Record<string, Partial<ChatSessionState>> };
    if (!p?.sessions) return current;
    const sessions: Record<string, ChatSessionState> = {};
    for (const [id, partial] of Object.entries(p.sessions)) {
      const base = createChatSessionState();
      sessions[id] = {
        ...base,
        ...partial,
        attachments: partial.attachments ? trimDraftAttachments(partial.attachments) : base.attachments,
      };
    }
    return { ...current, sessions };
  },
};
