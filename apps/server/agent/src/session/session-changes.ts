import type { SessionFileChange } from "@console/types";
import { getSessionDb } from "./session-helpers.js";
import type { StorageState } from "./utils.js";

function getSessionDbForSession(state: StorageState, sessionId: string) {
  const row = state.globalDb
    .prepare("SELECT project_id FROM sessions WHERE id = ?")
    .get(sessionId) as { project_id: string | null } | undefined;
  if (!row) return null;
  return getSessionDb(state, sessionId, row.project_id);
}

export function recordFileChange(
  state: StorageState,
  sessionId: string,
  change: SessionFileChange,
): void {
  const sessionDb = getSessionDbForSession(state, sessionId);
  if (!sessionDb) return;

  const now = change.updatedAt || Date.now();

  sessionDb
    .prepare(
      `INSERT INTO session_file_changes (path, turn_index, status, additions, deletions, diff_text, updated_at)
       VALUES (?, ?, ?, ?, ?, ?, ?)
       ON CONFLICT(path, turn_index) DO UPDATE SET
         status = excluded.status,
         additions = excluded.additions,
         deletions = excluded.deletions,
         diff_text = excluded.diff_text,
         updated_at = excluded.updated_at`,
    )
    .run(
      change.path,
      change.turnIndex || 0,
      change.status,
      change.additions || 0,
      change.deletions || 0,
      change.diffText || null,
      now,
    );
}

export function getSessionFileChanges(
  state: StorageState,
  sessionId: string,
  turnIndex?: number,
): SessionFileChange[] {
  const sessionDb = getSessionDbForSession(state, sessionId);
  if (!sessionDb) return [];

  let query = `SELECT path, status, additions, deletions, turn_index as turnIndex, diff_text as diffText, updated_at as updatedAt
       FROM session_file_changes`;
  const params: any[] = [];

  if (turnIndex !== undefined) {
    query += ` WHERE turn_index = ?`;
    params.push(turnIndex);
  }

  query += ` ORDER BY updated_at DESC`;

  const rows = sessionDb.prepare(query).all(...params) as SessionFileChange[];
  return rows;
}

export function clearSessionFileChanges(
  state: StorageState,
  sessionId: string,
): void {
  const sessionDb = getSessionDbForSession(state, sessionId);
  if (!sessionDb) return;

  sessionDb.prepare(`DELETE FROM session_file_changes`).run();
}
