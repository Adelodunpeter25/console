// Centralized draft service: syncing, loading, and debouncing composer drafts.

import { Console } from "console-nsh/native";

export interface ChatDraftData {
  sessionId: string;
  text: string;
  updatedAt: number;
}

const memoryDrafts = new Map<string, string>();
const saveTimers = new Map<string, ReturnType<typeof setTimeout>>();

async function loadDraft(baseUrl: string, sessionId: string): Promise<string> {
  const cached = memoryDrafts.get(sessionId);
  if (cached !== undefined) return cached;

  try {
    const raw = await Console.draft(baseUrl, sessionId);
    if (raw && raw !== "null") {
      const parsed = JSON.parse(raw) as { text?: string };
      const text = parsed.text ?? "";
      memoryDrafts.set(sessionId, text);
      return text;
    }
  } catch {
    // Best-effort
  }
  return "";
}

function getCachedDraft(sessionId: string): string {
  return memoryDrafts.get(sessionId) ?? "";
}

async function saveDraft(
  baseUrl: string,
  sessionId: string,
  text: string,
  immediate = false,
): Promise<void> {
  memoryDrafts.set(sessionId, text);

  const existingTimer = saveTimers.get(sessionId);
  if (existingTimer) {
    clearTimeout(existingTimer);
    saveTimers.delete(sessionId);
  }

  const persist = async () => {
    try {
      await Console.saveDraft(baseUrl, {
        sessionId,
        text,
        updatedAt: Date.now(),
      });
    } catch {
      // Best-effort
    }
  };

  if (immediate) {
    await persist();
  } else {
    const timer = setTimeout(() => {
      saveTimers.delete(sessionId);
      void persist();
    }, 300);
    saveTimers.set(sessionId, timer);
  }
}

export const DraftService = {
  load: loadDraft,
  get: getCachedDraft,
  save: saveDraft,
};
