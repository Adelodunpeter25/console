/**
 * Queued-prompt persistence: at most one queued prompt per session, staged
 * to auto-run once the session's active turn settles (or to steer it early).
 * Mirrors the `session-todos.ts` single-table pattern.
 */
import type { QueuedPrompt } from "@console/types";
import { getProjectIdBySessionId, getSessionDb } from "./session-helpers.js";
import type { StorageState } from "./utils.js";

interface QueuedPromptRow {
  queue_id: string;
  prompt: string;
  attachments: string | null;
  model_id: string | null;
  provider: string | null;
  approval_mode: string | null;
  created_at: number;
}

function rowToQueuedPrompt(sessionId: string, row: QueuedPromptRow): QueuedPrompt {
  let attachments: QueuedPrompt["attachments"];
  if (row.attachments) {
    try {
      attachments = JSON.parse(row.attachments);
    } catch {
      attachments = undefined;
    }
  }
  return {
    id: row.queue_id,
    sessionId,
    prompt: row.prompt,
    attachments,
    modelId: row.model_id ?? undefined,
    provider: (row.provider as QueuedPrompt["provider"]) ?? undefined,
    approvalMode: (row.approval_mode as QueuedPrompt["approvalMode"]) ?? undefined,
    createdAt: new Date(row.created_at).toISOString(),
  };
}

export function saveQueuedPrompt(
  state: StorageState,
  sessionId: string,
  queuedPrompt: QueuedPrompt,
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const now = Date.parse(queuedPrompt.createdAt) || Date.now();

  sessionDb
    .prepare(
      `INSERT INTO session_queued_prompt (id, queue_id, prompt, attachments, model_id, provider, approval_mode, created_at)
       VALUES (1, ?, ?, ?, ?, ?, ?, ?)
       ON CONFLICT(id) DO UPDATE SET
         queue_id = excluded.queue_id,
         prompt = excluded.prompt,
         attachments = excluded.attachments,
         model_id = excluded.model_id,
         provider = excluded.provider,
         approval_mode = excluded.approval_mode,
         created_at = excluded.created_at`,
    )
    .run(
      queuedPrompt.id,
      queuedPrompt.prompt,
      queuedPrompt.attachments && queuedPrompt.attachments.length > 0
        ? JSON.stringify(queuedPrompt.attachments)
        : null,
      queuedPrompt.modelId ?? null,
      queuedPrompt.provider ?? null,
      queuedPrompt.approvalMode ?? null,
      now,
    );
}

export function getQueuedPrompt(state: StorageState, sessionId: string): QueuedPrompt | null {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return null;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const row = sessionDb
    .prepare(
      "SELECT queue_id, prompt, attachments, model_id, provider, approval_mode, created_at FROM session_queued_prompt WHERE id = 1",
    )
    .get() as QueuedPromptRow | undefined;

  return row ? rowToQueuedPrompt(sessionId, row) : null;
}

export function clearQueuedPrompt(state: StorageState, sessionId: string): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  sessionDb.run("DELETE FROM session_queued_prompt");
}
