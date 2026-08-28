import type { TodoItem } from "@console/types";
import { getProjectIdBySessionId, getSessionDb } from "./session-helpers.js";
import type { StorageState } from "./utils.js";

export function saveSessionTodos(
  state: StorageState,
  sessionId: string,
  items: readonly TodoItem[],
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const now = Date.now();

  sessionDb.transaction(() => {
    sessionDb.run("DELETE FROM session_todos");
    if (items.length === 0) return;

    const insert = sessionDb.prepare(
      "INSERT INTO session_todos (id, content, status, updated_at) VALUES (?, ?, ?, ?)",
    );
    for (const item of items) {
      insert.run(item.id, item.content, item.status, now);
    }
  })();
}

export function getSessionTodos(
  state: StorageState,
  sessionId: string,
): TodoItem[] {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return [];

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const rows = sessionDb
    .prepare("SELECT id, content, status FROM session_todos ORDER BY id ASC")
    .all() as { id: number; content: string; status: string }[];

  return rows.map((r) => ({
    id: r.id,
    content: r.content,
    status: r.status as TodoItem["status"],
  }));
}

export function clearSessionTodos(
  state: StorageState,
  sessionId: string,
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  sessionDb.run("DELETE FROM session_todos");
}
