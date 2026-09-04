import type { Database as DatabaseType } from "bun:sqlite";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { AgentMessage, SessionHeader } from "@/agent/src/types/index.js";
import { repairToolCallHistory } from "@/agent/src/utils/tool-history.js";
import { replaceMessages } from "./session-messages.js";
import {
  ensureDir,
  findSessionDbPath,
  getProjectIdBySessionId,
  getScratchSessionDbPath,
  getSessionDb,
  getSessionDbPath,
  removeDbFile,
} from "./session-helpers.js";
import { getScratchDir, getSessionScratchDir } from "./apppaths.js";
import { type SessionIndexRow, type SessionMetaRow, type StorageState } from "./utils.js";

export interface CreateSessionOptions {
  id?: string;
  title?: string;
  cwd: string;
  projectId?: string | null;
  modelId: string;
  provider: string;
  approvalMode?: string;
}

export function createSession(state: StorageState, options: CreateSessionOptions): SessionHeader {
  const { globalDb } = state;
  const id = options.id ?? crypto.randomUUID();
  const now = Date.now();
  const title = options.title?.trim() || "New Session";
  const projectId = options.projectId ?? null;
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
    projectId: projectId ?? undefined,
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
  options?: { limit?: number; before?: number },
): {
  header: SessionHeader;
  messages: AgentMessage[];
  hasMore: boolean;
  nextCursor: number | null;
} | null {
  const { globalDb, storageDir } = state;
  const indexRow = globalDb.prepare(`SELECT * FROM sessions WHERE id = ?`).get(sessionId) as
    | SessionIndexRow
    | undefined;

  if (indexRow && indexRow.deleted_at !== null && indexRow.deleted_at !== undefined) {
    return null;
  }

  let projectId: string | null = indexRow?.project_id && indexRow.project_id !== "scratch" ? indexRow.project_id : null;
  let dbPath: string | undefined;

  if (state.storageDir === ":memory:") {
    dbPath = undefined;
  } else if (projectId) {
    dbPath = getSessionDbPath(storageDir, projectId, sessionId);
  } else {
    dbPath = getScratchSessionDbPath(storageDir, sessionId);
    if (!fs.existsSync(dbPath)) {
      const found = findSessionDbPath(storageDir, sessionId);
      if (found) {
        dbPath = found;
        const sessionsIdx = dbPath.indexOf(`${path.sep}sessions${path.sep}`);
        if (sessionsIdx > 0) {
          const dir = path.basename(dbPath.slice(0, sessionsIdx));
          projectId = dir === "scratch" ? null : dir;
        }
      }
    }
  }

  const hasDbFile = state.sessionDbs.has(sessionId) || (!!dbPath && fs.existsSync(dbPath));
  if (!indexRow && !hasDbFile) return null;

  let meta: SessionMetaRow | null = null;
  let messages: AgentMessage[] = [];
  let hasMore = false;
  let nextCursor: number | null = null;
  let storedMessageCount: number | undefined = indexRow?.message_count;

  if (hasDbFile) {
    const sessionDb = getSessionDb(state, sessionId, projectId);
    meta =
      (sessionDb
        .prepare(
          `SELECT title, cwd, project_id, model_id, provider, approval_mode, repaired, created_at, updated_at FROM session_meta WHERE id = 1`,
        )
        .get() as SessionMetaRow | undefined) ?? null;

    if (options) {
      const limit = Math.max(1, Math.floor(options.limit ?? 50));
      const rows = (options.before === undefined
        ? sessionDb
            .prepare(
              `SELECT content, created_at, rowid AS rowid FROM messages ORDER BY rowid DESC LIMIT ?`,
            )
            .all(limit + 1)
        : sessionDb
            .prepare(
              `SELECT content, created_at, rowid AS rowid FROM messages WHERE rowid < ? ORDER BY rowid DESC LIMIT ?`,
            )
            .all(options.before, limit + 1)) as Array<{
        content: string;
        created_at: number;
        rowid: number;
      }>;

      hasMore = rows.length > limit;
      const pageRows = (hasMore ? rows.slice(0, limit) : rows).reverse();
      messages = pageRows.map((r) => {
        const msg = JSON.parse(r.content) as AgentMessage;
        msg.createdAt = r.created_at;
        return msg;
      });
      nextCursor = hasMore ? (pageRows[0]?.rowid ?? null) : null;

      if (storedMessageCount === undefined) {
        const countRow = sessionDb.prepare(`SELECT COUNT(*) AS count FROM messages`).get() as {
          count: number;
        };
        storedMessageCount = countRow.count;
      }
    } else {
      const messageRows = sessionDb
        .prepare(`SELECT content, created_at FROM messages ORDER BY rowid ASC`)
        .all() as Array<{ content: string; created_at: number }>;
      messages = messageRows.map((r) => {
        const msg = JSON.parse(r.content) as AgentMessage;
        msg.createdAt = r.created_at;
        return msg;
      });
    }
  }

  const title = meta?.title ?? indexRow?.title ?? "New Session";
  const cwd = meta?.cwd ?? indexRow?.cwd ?? process.cwd();
  const resolvedProjectId = meta?.project_id ?? projectId ?? indexRow?.project_id ?? null;
  const modelId = meta?.model_id ?? indexRow?.model_id ?? "claude-opus-4-6-thinking";
  const provider = meta?.provider ?? indexRow?.provider ?? "antigravity";
  const approvalMode = meta?.approval_mode ?? indexRow?.approval_mode ?? "always-ask";
  const createdAt = meta?.created_at ?? indexRow?.created_at ?? Date.now();
  const updatedAt = meta?.updated_at ?? indexRow?.updated_at ?? createdAt;
  // The status column is read back as a raw string from SQLite; coerce to the
  // SessionStatus union (defensive: unknown values fall back to "idle").
  const sessionStatus =
    indexRow?.status === "working" ||
    indexRow?.status === "done" ||
    indexRow?.status === "needs_attention"
      ? indexRow.status
      : ("idle" as const);

  if (!indexRow) {
    globalDb
      .prepare(
        `INSERT INTO sessions
          (id, title, cwd, project_id, model_id, provider, message_count, approval_mode, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      )
      .run(
        sessionId,
        title,
        cwd,
        resolvedProjectId,
        modelId,
        provider,
        storedMessageCount ?? messages.length,
        approvalMode,
        createdAt,
        updatedAt,
      );
  }

  return {
    header: {
      id: sessionId,
      title,
      cwd,
      projectId: resolvedProjectId ?? undefined,
      modelId,
      provider,
      approvalMode,
      createdAt,
      updatedAt,
      messageCount: storedMessageCount ?? messages.length,
      status: sessionStatus,
    },
    messages,
    hasMore,
    nextCursor,
  };
}

/** Repair an interrupted tool history once, then remember that the check ran. */
export function repairSession(state: StorageState, sessionId: string): boolean {
  let projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return false;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const meta = sessionDb.prepare(`SELECT repaired FROM session_meta WHERE id = 1`).get() as
    | { repaired: number }
    | undefined;
  if (!meta || meta.repaired === 1) return false;

  const session = loadSession(state, sessionId);
  if (!session) return false;

  const repaired = repairToolCallHistory(session.messages);
  if (repaired.repaired) {
    replaceMessages(state, sessionId, repaired.messages);
  }
  sessionDb.prepare(`UPDATE session_meta SET repaired = 1 WHERE id = 1`).run();
  return true;
}

/** Mark a session dirty before a new run so its final history is checked. */
export function markSessionNeedsRepair(state: StorageState, sessionId: string): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  sessionDb.prepare(`UPDATE session_meta SET repaired = 0 WHERE id = 1`).run();
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
    projectId: r.project_id ?? undefined,
    modelId: r.model_id,
    provider: r.provider,
    createdAt: r.created_at,
    updatedAt: r.updated_at,
    messageCount: r.message_count,
    approvalMode: r.approval_mode ?? "always-ask",
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

  // Cleanup scratch working directory for scratchpad sessions
  if (row.project_id == null) {
    try {
      const scratchDir = getSessionScratchDir(sessionId);
      // Safety: only delete if under the scratch root
      const scratchRoot = getScratchDir();
      if (scratchDir.startsWith(scratchRoot) && fs.existsSync(scratchDir)) {
        fs.rmSync(scratchDir, { recursive: true, force: true });
      }
    } catch {
      // Ignored
    }
  }

  const info = globalDb.prepare(`DELETE FROM sessions WHERE id = ?`).run(sessionId);
  return info.changes > 0;
}

export function updateTitle(state: StorageState, sessionId: string, title: string): boolean {
  const { globalDb, storageDir } = state;
  const projectId = getProjectIdBySessionId(globalDb, sessionId);
  const now = Date.now();
  const trimmed = title.trim();

  {
    const dbPath = projectId == null ? getScratchSessionDbPath(storageDir, sessionId) : getSessionDbPath(storageDir, projectId, sessionId);
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

  {
    const dbPath = projectId == null ? getScratchSessionDbPath(storageDir, sessionId) : getSessionDbPath(storageDir, projectId, sessionId);
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

export function updateCwd(
  state: StorageState,
  sessionId: string,
  cwd: string,
  newProjectId?: string | null,
): boolean {
  const { globalDb, storageDir } = state;
  const oldProjectId = getProjectIdBySessionId(globalDb, sessionId) ?? null;
  const targetProjectId =
    newProjectId !== undefined
      ? newProjectId === "scratch"
        ? null
        : (newProjectId ?? null)
      : oldProjectId;
  const now = Date.now();
  const trimmed = cwd.trim();

  {
    const dbPath =
      oldProjectId == null
        ? getScratchSessionDbPath(storageDir, sessionId)
        : getSessionDbPath(storageDir, oldProjectId, sessionId);
    if (state.sessionDbs.has(sessionId) || (storageDir !== ":memory:" && fs.existsSync(dbPath))) {
      const sessionDb = getSessionDb(state, sessionId, oldProjectId);
      sessionDb
        .prepare(`UPDATE session_meta SET cwd = ?, project_id = ?, updated_at = ? WHERE id = 1`)
        .run(trimmed, targetProjectId, now);
    }
  }

  const info = globalDb
    .prepare(`UPDATE sessions SET cwd = ?, project_id = ?, updated_at = ? WHERE id = ?`)
    .run(trimmed, targetProjectId, now, sessionId);
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

  {
    const dbPath = projectId == null ? getScratchSessionDbPath(storageDir, sessionId) : getSessionDbPath(storageDir, projectId, sessionId);
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
