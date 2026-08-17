import { type Database as DatabaseType } from "better-sqlite3";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { AgentMessage, SessionHeader } from "../types/index.js";
import {
  findSessionDbPath,
  getProjectIdBySessionId,
  getSessionDb,
  getSessionDbPath,
  removeDbFile,
} from "./session-helpers.js";
import { type SessionIndexRow, type SessionMetaRow, type StorageState } from "./utils.js";

export interface CreateSessionOptions {
  id?: string;
  title?: string;
  cwd: string;
  projectId?: string;
  modelId: string;
  provider: string;
  approvalMode?: string;
}

export function createSession(state: StorageState, options: CreateSessionOptions): SessionHeader {
  const { globalDb } = state;
  const id = options.id ?? crypto.randomUUID();
  const now = Date.now();
  const title = options.title?.trim() || "New Session";
  const projectId = options.projectId || "default";
  const approvalMode = options.approvalMode ?? "always-ask";

  // 1. Write header to the global index.
  globalDb
    .prepare(
      `INSERT INTO sessions
        (id, title, cwd, project_id, model_id, provider, message_count, approval_mode, created_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
    )
    .run(
      id,
      title,
      options.cwd,
      projectId,
      options.modelId,
      options.provider,
      approvalMode,
      now,
      now,
    );

  // 2. Initialize the per-session DB with the authoritative meta row.
  const sessionDb = getSessionDb(state, id, projectId);
  sessionDb
    .prepare(
      `INSERT INTO session_meta
        (id, title, cwd, project_id, model_id, provider, approval_mode, created_at, updated_at)
       VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)`,
    )
    .run(title, options.cwd, projectId, options.modelId, options.provider, approvalMode, now, now);

  return {
    id,
    title,
    cwd: options.cwd,
    modelId: options.modelId,
    provider: options.provider,
    approvalMode,
    createdAt: now,
    updatedAt: now,
    messageCount: 0,
    status: "idle",
  };
}

export function loadSession(
  state: StorageState,
  sessionId: string,
): { header: SessionHeader; messages: AgentMessage[] } | null {
  const { globalDb, storageDir } = state;
  const indexRow = globalDb.prepare(`SELECT * FROM sessions WHERE id = ?`).get(sessionId) as
    | SessionIndexRow
    | undefined;

  if (indexRow && indexRow.deleted_at !== null && indexRow.deleted_at !== undefined) {
    return null;
  }

  let projectId: string | null = indexRow?.project_id ?? null;
  let dbPath: string | undefined;

  if (projectId) {
    dbPath = getSessionDbPath(storageDir, projectId, sessionId);
  } else {
    dbPath = findSessionDbPath(storageDir, sessionId);
    if (dbPath) {
      const sessionsIdx = dbPath.indexOf(`${path.sep}sessions${path.sep}`);
      if (sessionsIdx > 0) {
        projectId = path.basename(dbPath.slice(0, sessionsIdx));
      }
    }
  }

  const hasDbFile = !!dbPath && fs.existsSync(dbPath);
  if (!indexRow && !hasDbFile) return null;

  let meta: SessionMetaRow | null = null;
  let messages: AgentMessage[] = [];

  if (hasDbFile && projectId) {
    const sessionDb = getSessionDb(state, sessionId, projectId);
    meta =
      (sessionDb
        .prepare(
          `SELECT title, cwd, project_id, model_id, provider, approval_mode, created_at, updated_at FROM session_meta WHERE id = 1`,
        )
        .get() as SessionMetaRow | undefined) ?? null;

    const messageRows = sessionDb
      .prepare(`SELECT content, created_at FROM messages ORDER BY rowid ASC`)
      .all() as Array<{ content: string; created_at: number }>;
    messages = messageRows.map((r) => {
      const msg = JSON.parse(r.content) as AgentMessage;
      msg.createdAt = r.created_at;
      return msg;
    });
  }

  const title = meta?.title ?? indexRow?.title ?? "New Session";
  const cwd = meta?.cwd ?? indexRow?.cwd ?? process.cwd();
  const resolvedProjectId = meta?.project_id ?? projectId ?? indexRow?.project_id ?? "default";
  const modelId = meta?.model_id ?? indexRow?.model_id ?? "gemini-2.5-pro";
  const provider = meta?.provider ?? indexRow?.provider ?? "antigravity";
  const approvalMode = meta?.approval_mode ?? indexRow?.approval_mode ?? "always-ask";
  const createdAt = meta?.created_at ?? indexRow?.created_at ?? Date.now();
  const updatedAt = meta?.updated_at ?? indexRow?.updated_at ?? createdAt;

  if (!indexRow) {
    globalDb
      .prepare(
        `INSERT INTO sessions
          (id, title, cwd, project_id, model_id, provider, message_count, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      )
      .run(
        sessionId,
        title,
        cwd,
        resolvedProjectId,
        modelId,
        provider,
        messages.length,
        createdAt,
        updatedAt,
      );
  }

  return {
    header: {
      id: sessionId,
      title,
      cwd,
      modelId,
      provider,
      approvalMode,
      createdAt,
      updatedAt,
      messageCount: messages.length,
      status: indexRow?.status ?? "idle",
    },
    messages,
  };
}

export function listSessions(
  globalDb: DatabaseType,
  options?: { cwd?: string; projectId?: string; limit?: number; onlyDeleted?: boolean },
): SessionHeader[] {
  const limit = options?.limit ?? 100;
  const onlyDeleted = !!options?.onlyDeleted;
  const deletedCondition = onlyDeleted ? "deleted_at IS NOT NULL" : "deleted_at IS NULL";

  let rows: any[];
  if (options?.cwd) {
    rows = globalDb
      .prepare(`SELECT * FROM sessions WHERE cwd = ? AND ${deletedCondition} ORDER BY updated_at DESC LIMIT ?`)
      .all(options.cwd, limit);
  } else if (options?.projectId) {
    rows = globalDb
      .prepare(`SELECT * FROM sessions WHERE project_id = ? AND ${deletedCondition} ORDER BY updated_at DESC LIMIT ?`)
      .all(options.projectId, limit);
  } else {
    rows = globalDb
      .prepare(`SELECT * FROM sessions WHERE ${deletedCondition} ORDER BY updated_at DESC LIMIT ?`)
      .all(limit);
  }

  return rows.map((r) => ({
    id: r.id,
    title: r.title,
    cwd: r.cwd,
    modelId: r.model_id,
    provider: r.provider,
    createdAt: r.created_at,
    updatedAt: r.updated_at,
    messageCount: r.message_count,
    status: r.status ?? "idle",
    deletedAt: r.deleted_at ?? undefined,
  }));
}

export function deleteSession(state: StorageState, sessionId: string): boolean {
  const { globalDb } = state;

  const db = state.sessionDbs.get(sessionId);
  if (db) {
    try {
      db.close();
    } catch {
      // Ignored
    }
    state.sessionDbs.delete(sessionId);
  }

  const now = Date.now();
  const info = globalDb.prepare(`UPDATE sessions SET deleted_at = ? WHERE id = ?`).run(now, sessionId);
  return info.changes > 0;
}

export function restoreSession(state: StorageState, sessionId: string): boolean {
  const { globalDb } = state;
  const info = globalDb.prepare(`UPDATE sessions SET deleted_at = NULL WHERE id = ?`).run(sessionId);
  return info.changes > 0;
}

/** Permanently remove a session that has already been soft-deleted. */
export function permanentlyDeleteSession(state: StorageState, sessionId: string): boolean {
  const { globalDb, storageDir } = state;
  const row = globalDb
    .prepare(`SELECT project_id, deleted_at FROM sessions WHERE id = ?`)
    .get(sessionId) as { project_id: string | null; deleted_at: number | null } | undefined;

  if (!row || row.deleted_at === null || row.deleted_at === undefined) return false;

  const db = state.sessionDbs.get(sessionId);
  if (db) {
    try {
      db.close();
    } catch {
      // Ignored — the database file can still be removed below.
    }
    state.sessionDbs.delete(sessionId);
  }

  const dbPath = row.project_id
    ? getSessionDbPath(storageDir, row.project_id, sessionId)
    : findSessionDbPath(storageDir, sessionId);
  if (dbPath) removeDbFile(dbPath);

  const info = globalDb.prepare(`DELETE FROM sessions WHERE id = ?`).run(sessionId);
  return info.changes > 0;
}

export function updateTitle(state: StorageState, sessionId: string, title: string): boolean {
  const { globalDb, storageDir } = state;
  const projectId = getProjectIdBySessionId(globalDb, sessionId);
  const now = Date.now();
  const trimmed = title.trim();

  if (projectId) {
    const dbPath = getSessionDbPath(storageDir, projectId, sessionId);
    if (state.sessionDbs.has(sessionId) || fs.existsSync(dbPath)) {
      const sessionDb = getSessionDb(state, sessionId, projectId);
      sessionDb
        .prepare(`UPDATE session_meta SET title = ?, updated_at = ? WHERE id = 1`)
        .run(trimmed, now);
    }
  }

  const info = globalDb
    .prepare(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`)
    .run(trimmed, now, sessionId);
  return info.changes > 0;
}

export function updateModel(
  state: StorageState,
  sessionId: string,
  modelId: string,
  provider: string,
): boolean {
  const { globalDb, storageDir } = state;
  const projectId = getProjectIdBySessionId(globalDb, sessionId);
  const now = Date.now();

  if (projectId) {
    const dbPath = getSessionDbPath(storageDir, projectId, sessionId);
    if (state.sessionDbs.has(sessionId) || fs.existsSync(dbPath)) {
      const sessionDb = getSessionDb(state, sessionId, projectId);
      sessionDb
        .prepare(`UPDATE session_meta SET model_id = ?, provider = ?, updated_at = ? WHERE id = 1`)
        .run(modelId, provider, now);
    }
  }

  const info = globalDb
    .prepare(`UPDATE sessions SET model_id = ?, provider = ?, updated_at = ? WHERE id = ?`)
    .run(modelId, provider, now, sessionId);
  return info.changes > 0;
}

export function updateCwd(state: StorageState, sessionId: string, cwd: string): boolean {
  const { globalDb, storageDir } = state;
  const projectId = getProjectIdBySessionId(globalDb, sessionId);
  const now = Date.now();
  const trimmed = cwd.trim();

  if (projectId) {
    const dbPath = getSessionDbPath(storageDir, projectId, sessionId);
    if (state.sessionDbs.has(sessionId) || fs.existsSync(dbPath)) {
      const sessionDb = getSessionDb(state, sessionId, projectId);
      sessionDb
        .prepare(`UPDATE session_meta SET cwd = ?, updated_at = ? WHERE id = 1`)
        .run(trimmed, now);
    }
  }

  const info = globalDb
    .prepare(`UPDATE sessions SET cwd = ?, updated_at = ? WHERE id = ?`)
    .run(trimmed, now, sessionId);
  return info.changes > 0;
}

export function updateApprovalMode(
  state: StorageState,
  sessionId: string,
  approvalMode: string,
): boolean {
  const { globalDb, storageDir } = state;
  const projectId = getProjectIdBySessionId(globalDb, sessionId);
  const now = Date.now();

  if (projectId) {
    const dbPath = getSessionDbPath(storageDir, projectId, sessionId);
    if (state.sessionDbs.has(sessionId) || fs.existsSync(dbPath)) {
      const sessionDb = getSessionDb(state, sessionId, projectId);
      sessionDb
        .prepare(`UPDATE session_meta SET approval_mode = ?, updated_at = ? WHERE id = 1`)
        .run(approvalMode, now);
    }
  }

  const info = globalDb
    .prepare(`UPDATE sessions SET approval_mode = ?, updated_at = ? WHERE id = ?`)
    .run(approvalMode, now, sessionId);
  return info.changes > 0;
}

export function updateSessionStatus(
  globalDb: DatabaseType,
  sessionId: string,
  status: string,
): void {
  const now = Date.now();
  globalDb
    .prepare(`UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?`)
    .run(status, now, sessionId);
}

export function clearAll(state: StorageState): void {
  state.globalDb.exec(`DELETE FROM sessions; DELETE FROM projects;`);
  for (const [, db] of state.sessionDbs) {
    try {
      db.exec(`DELETE FROM messages; DELETE FROM session_meta;`);
    } catch {
      // Ignored
    }
  }
}
