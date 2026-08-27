import type { SessionFileChange } from "@console/types";
import { getProjectIdBySessionId, getSessionDb } from "./session-helpers.js";
import type { StorageState } from "./utils.js";

export function recordFileChange(
  state: StorageState,
  sessionId: string,
  change: SessionFileChange,
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
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
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return [];

  const sessionDb = getSessionDb(state, sessionId, projectId);
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
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  sessionDb.prepare(`DELETE FROM session_file_changes`).run();
}
