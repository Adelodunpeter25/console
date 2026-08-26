import type { Observable } from "@legendapp/state";
import type { ImageAttachment } from "@console/types";
import type { ChatSessionState } from "@/types";
import { createChatSessionState } from "@/types/chat-state";
import { mmkvStringStorage } from "@/utils/storage";
import { hasPersistableDraft, trimDraftAttachments } from "./draft";
import { ensureMessageIds } from "@/utils/chat-events";

/**
 * Persistence for the Legend-State chat store.
 *
 * Keeps the exact same on-disk format the zustand `persist` middleware wrote
 * ({ state: { sessions }, version }) so upgrades and downgrades are lossless.
 * Writes are throttled while streaming is suppressed via `setSuppressPersist`.
 *
 * Receives the sessions observable as a parameter (rather than importing it
 * from useChatStore) to avoid a require cycle.
 */

/** The sessions node being persisted; bound by initChatPersistence(). */
let sessionsNode: Observable<Record<string, ChatSessionState>> | null = null;

export const PERSIST_NAME = "console-chat-cache";
export const PERSIST_VERSION = 1;
export const MAX_PERSISTED_SESSIONS = 25;
export const MAX_PERSISTED_MESSAGES = 50;

const SAVE_THROTTLE_MS = 300;

// --- Streaming persist suppression ---
let _suppressPersist = false;
let _savePendingWhileSuppressed = false;
let _saveTimer: ReturnType<typeof setTimeout> | null = null;

/** Call with true when streaming starts, false when it ends (triggers a flush). */
export function setSuppressPersist(suppress: boolean): void {
  _suppressPersist = suppress;
  if (!suppress && _savePendingWhileSuppressed) {
    _savePendingWhileSuppressed = false;
    saveNow();
  }
}

/** Fields persisted per session — anything else is dropped on rehydrate. */
function sanitizeSessionPartial(partial: unknown): Partial<ChatSessionState> | null {
  if (typeof partial !== "object" || partial === null) return null;
  const p = partial as Record<string, unknown>;
  return {
    // Only known, expected-typed fields are copied — blind spreads would let
    // corrupt/stale persisted JSON inject unexpected props into app state.
    messages: Array.isArray(p.messages)
      ? ensureMessageIds(
          p.messages.filter((m): m is ChatSessionState["messages"][number] => typeof m === "object" && m !== null),
        )
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
  const sessions = (persisted ?? {}) as Record<string, unknown>;
  if (typeof sessions !== "object" || sessions === null) return {};
  const out: Record<string, ChatSessionState> = {};
  for (const [id, raw] of Object.entries(sessions)) {
    const partial = sanitizeSessionPartial(raw);
    if (!partial) continue;
    out[id] = { ...createChatSessionState(), ...partial };
  }
  return out;
}

/** Build the capped/partialized payload — same policy as the old partialize. */
function buildPersistedSessions(): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(sessionsNode?.peek() ?? {})
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
  );
}

function saveNow(): void {
  try {
    const payload = JSON.stringify({
      state: { sessions: buildPersistedSessions() },
      version: PERSIST_VERSION,
    });
    mmkvStringStorage.setItem(PERSIST_NAME, payload);
  } catch (err) {
    console.warn("Could not persist chat cache:", err);
  }
}

function scheduleSave(): void {
  if (_suppressPersist) {
    _savePendingWhileSuppressed = true;
    return;
  }
  if (_saveTimer != null) return; // trailing throttle already armed
  _saveTimer = setTimeout(() => {
    _saveTimer = null;
    saveNow();
  }, SAVE_THROTTLE_MS);
}

/** Wipe the persisted cache (backend switch / disconnect). */
export function clearChatStorage(): void {
  try {
    mmkvStringStorage.removeItem(PERSIST_NAME);
  } catch (err) {
    console.warn("Could not clear persisted chat storage:", err);
  }
}

/**
 * Hydrate the sessions node from storage and start persisting changes.
 * Called once at module load of useChatStore (before first render).
 */
export function initChatPersistence(
  sessions: Observable<Record<string, ChatSessionState>>,
): void {
  sessionsNode = sessions;

  // --- Hydrate ---
  try {
    const raw = mmkvStringStorage.getItem(PERSIST_NAME);
    if (raw) {
      const parsed = JSON.parse(raw) as {
        state?: { sessions?: unknown };
        version?: number;
      };
      // Payloads from older/unknown versions are re-validated field-by-field
      // by sanitize; nothing to structurally migrate yet.
      sessions.set(sanitizePersistedSessions(parsed.state?.sessions));
    }
  } catch (err) {
    console.warn("Could not restore persisted chats:", err);
  }

  // --- Persist on change ---
  sessions.onChange(scheduleSave);
}
