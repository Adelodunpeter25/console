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
      `INSERT INTO session_file_changes (path, status, additions, deletions, turn_index, updated_at)
       VALUES (?, ?, ?, ?, ?, ?)
       ON CONFLICT(path) DO UPDATE SET
         status = excluded.status,
         additions = excluded.additions,
         deletions = excluded.deletions,
         turn_index = excluded.turn_index,
         updated_at = excluded.updated_at`,
    )
    .run(
      change.path,
      change.status,
      change.additions || 0,
      change.deletions || 0,
      change.turnIndex || 0,
      now,
    );
}

export function getSessionFileChanges(
  state: StorageState,
  sessionId: string,
): SessionFileChange[] {
  const sessionDb = getSessionDbForSession(state, sessionId);
  if (!sessionDb) return [];

  const rows = sessionDb
    .prepare(
      `SELECT path, status, additions, deletions, turn_index as turnIndex, updated_at as updatedAt
       FROM session_file_changes
       ORDER BY updated_at DESC`,
    )
    .all() as SessionFileChange[];

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
