import { createJSONStorage, type PersistOptions } from "zustand/middleware";
import type { ImageAttachment } from "@console/types";
import type { ChatSessionState } from "@/types";
import { createChatSessionState } from "@/types/chat-state";
import { mmkvZustandStorage } from "@/utils/storage";
import { hasPersistableDraft, trimDraftAttachments } from "./draft";
import type { ChatStoreState } from "@/stores/useChatStore";

export const PERSIST_NAME = "console-chat-cache";
export const PERSIST_VERSION = 1;
export const MAX_PERSISTED_SESSIONS = 25;
export const MAX_PERSISTED_MESSAGES = 50;

/**
 * Shape of what partialize writes: per session only a subset of
 * ChatSessionState fields is persisted.
 */
export interface ChatStorePersistedState {
  sessions: Record<string, Partial<ChatSessionState>>;
}

/** Fields persisted per session — anything else is dropped on rehydrate. */
function sanitizeSessionPartial(partial: unknown): Partial<ChatSessionState> | null {
  if (typeof partial !== "object" || partial === null) return null;
  const p = partial as Record<string, unknown>;
  return {
    // Only known, expected-typed fields are copied — blind spreads would let
    // corrupt/stale persisted JSON inject unexpected props into app state.
    messages: Array.isArray(p.messages)
      ? p.messages.filter((m): m is ChatSessionState["messages"][number] => typeof m === "object" && m !== null)
      : [],
    runs: Array.isArray(p.runs)
      ? p.runs.filter((r): r is ChatSessionState["runs"][number] => typeof r === "object" && r !== null)
      : [],
    input: typeof p.input === "string" ? p.input : "",
    attachments: Array.isArray(p.attachments)
      ? trimDraftAttachments(p.attachments.filter((a): a is ImageAttachment => typeof a === "object" && a !== null))
      : [],
    draftUpdatedAt: typeof p.draftUpdatedAt === "number" ? p.draftUpdatedAt : undefined,
  };
}

/** Validates an unknown persisted payload, returning only well-formed sessions. */
function sanitizePersistedSessions(persisted: unknown): Record<string, ChatSessionState> {
  const p = persisted as { sessions?: unknown } | null;
  if (typeof p?.sessions !== "object" || p.sessions === null) return {};
  const sessions: Record<string, ChatSessionState> = {};
  for (const [id, raw] of Object.entries(p.sessions)) {
    const partial = sanitizeSessionPartial(raw);
    if (!partial) continue;
    sessions[id] = { ...createChatSessionState(), ...partial };
  }
  return sessions;
}

export const chatPersistConfig: PersistOptions<ChatStoreState, ChatStorePersistedState> = {
  name: PERSIST_NAME,
  version: PERSIST_VERSION,
  storage: createJSONStorage(() => mmkvZustandStorage),
  // Payloads from older/unknown versions are re-validated field-by-field by
  // merge below; nothing to structurally migrate yet.
  migrate: (persisted) => persisted as ChatStorePersistedState,
  partialize: (state) => ({
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
  merge: (persisted, current) => {
    const sessions = sanitizePersistedSessions(persisted);
    return { ...current, sessions };
  },
};
