/**
 * Session CRUD operations spanning the global index and per-session DBs.
 *
 * Each session is fully self-contained in its own SQLite file at
 * `<storage>/projects/<projectId>/sessions/<sessionId>.db`. The global
 * `sessions` table mirrors headers for fast listing.
 */
import DatabaseConstructor, { type Database as DatabaseType } from "better-sqlite3";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { AgentMessage, SessionHeader } from "../types/index.js";
import { initSessionDatabase } from "./schema.js";
import { truncateForPersistence, type SessionIndexRow, type SessionMetaRow, type StorageState } from "./utils.js";

// ----------------------------------------------------------------------
// Path helpers
// ----------------------------------------------------------------------

function getProjectStorageDir(storageDir: string, projectId: string): string {
  return path.join(storageDir, "projects", projectId);
}

function getProjectSessionsDir(storageDir: string, projectId: string): string {
  return path.join(getProjectStorageDir(storageDir, projectId), "sessions");
}

function getSessionDbPath(storageDir: string, projectId: string, sessionId: string): string {
  return path.join(getProjectSessionsDir(storageDir, projectId), `${sessionId}.db`);
}

function ensureDir(filePath: string): void {
  try {
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
  } catch {
    // Ignored if folder exists
  }
}

function removeDbFile(dbPath: string): void {
  try {
    if (fs.existsSync(dbPath)) fs.unlinkSync(dbPath);
    for (const ext of ["-wal", "-shm"]) {
      const sidecar = `${dbPath}${ext}`;
      if (fs.existsSync(sidecar)) fs.unlinkSync(sidecar);
    }
  } catch {
    // Ignored
  }
}

/**
 * Scan all project session directories for an orphaned session DB file.
 * Used when the global index is missing but the file still exists.
 */
function findSessionDbPath(storageDir: string, sessionId: string): string | undefined {
  const projectsRoot = path.join(storageDir, "projects");
  let projects: string[] = [];
  try {
    projects = fs
      .readdirSync(projectsRoot, { withFileTypes: true })
      .filter((d) => d.isDirectory())
      .map((d) => d.name);
  } catch {
    // No projects directory yet.
  }

  for (const projId of projects) {
    const candidate = getSessionDbPath(storageDir, projId, sessionId);
    if (fs.existsSync(candidate)) return candidate;
  }
  return undefined;
}

// ----------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------

function getProjectIdBySessionId(globalDb: DatabaseType, sessionId: string): string | null {
  const row = globalDb
    .prepare("SELECT project_id FROM sessions WHERE id = ?")
    .get(sessionId) as { project_id: string } | undefined;
  return row ? row.project_id : null;
}

function getSessionDb(
  state: StorageState,
  sessionId: string,
  projectId: string,
): DatabaseType {
  let db = state.sessionDbs.get(sessionId);
  if (!db) {
    const dbPath = getSessionDbPath(state.storageDir, projectId, sessionId);
    ensureDir(dbPath);
    db = new DatabaseConstructor(dbPath);
    initSessionDatabase(db);
    state.sessionDbs.set(sessionId, db);
  }
  return db;
}

function bumpSessionUpdated(
  globalDb: DatabaseType,
  sessionId: string,
  now: number,
  added: number,
): void {
  globalDb
    .prepare(`UPDATE sessions SET updated_at = ?, message_count = message_count + ? WHERE id = ?`)
    .run(now, added, sessionId);
}

// ----------------------------------------------------------------------
// Public operations
// ----------------------------------------------------------------------

interface CreateSessionOptions {
  id?: string;
  title?: string;
  cwd: string;
  projectId?: string;
  modelId: string;
  provider: string;
}

export function createSession(state: StorageState, options: CreateSessionOptions): SessionHeader {
  const { globalDb, storageDir } = state;
  const id = options.id ?? crypto.randomUUID();
  const now = Date.now();
  const title = options.title?.trim() || "New Session";
  const projectId = options.projectId || "default";

  // 1. Write header to the global index.
  globalDb
    .prepare(
      `INSERT INTO sessions
        (id, title, cwd, project_id, model_id, provider, message_count, created_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
    )
    .run(id, title, options.cwd, projectId, options.modelId, options.provider, now, now);

  // 2. Initialize the per-session DB with the authoritative meta row.
  const sessionDb = getSessionDb(state, id, projectId);
  sessionDb
    .prepare(
      `INSERT INTO session_meta
        (id, title, cwd, project_id, model_id, provider, created_at, updated_at)
       VALUES (1, ?, ?, ?, ?, ?, ?, ?)`,
    )
    .run(title, options.cwd, projectId, options.modelId, options.provider, now, now);

  return {
    id,
    title,
    cwd: options.cwd,
    modelId: options.modelId,
    provider: options.provider,
    createdAt: now,
    updatedAt: now,
    messageCount: 0,
    status: "idle",
  };
}

export function appendMessage(state: StorageState, sessionId: string, message: AgentMessage): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const now = Date.now();
  const safeMsg = truncateForPersistence(message);
  const msgId = (safeMsg as any).id || crypto.randomUUID();
  const contentJson = JSON.stringify(safeMsg);

  const sessionDb = getSessionDb(state, sessionId, projectId);
  sessionDb
    .prepare(`INSERT INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`)
    .run(msgId, safeMsg.role, contentJson, now);

  bumpSessionUpdated(state.globalDb, sessionId, now, 1);
}

export function appendMessages(
  state: StorageState,
  sessionId: string,
  messages: AgentMessage[],
): void {
  if (messages.length === 0) return;
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const now = Date.now();
  const sessionDb = getSessionDb(state, sessionId, projectId);
  const insertMsg = sessionDb.prepare(
    `INSERT OR IGNORE INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`,
  );

  let inserted = 0;
  const tx = sessionDb.transaction(() => {
    for (const msg of messages) {
      const safeMsg = truncateForPersistence(msg);
      const msgId =
        (safeMsg as any).id ||
        crypto.createHash("sha256").update(JSON.stringify(safeMsg)).digest("hex").slice(0, 32);
      const info = insertMsg.run(msgId, safeMsg.role, JSON.stringify(safeMsg), now);
      if (info.changes > 0) inserted++;
    }
  });
  tx();

  bumpSessionUpdated(state.globalDb, sessionId, now, inserted);
}

export function loadSession(
  state: StorageState,
  sessionId: string,
): { header: SessionHeader; messages: AgentMessage[] } | null {
  const { globalDb, storageDir } = state;
  const indexRow = globalDb
    .prepare(`SELECT * FROM sessions WHERE id = ?`)
    .get(sessionId) as SessionIndexRow | undefined;

  // Resolve projectId for routing.
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
    meta = (sessionDb
      .prepare(
        `SELECT title, cwd, project_id, model_id, provider, created_at, updated_at FROM session_meta WHERE id = 1`,
      )
      .get() as SessionMetaRow | undefined) ?? null;

    const messageRows = sessionDb
      .prepare(`SELECT content FROM messages ORDER BY created_at ASC, rowid ASC`)
      .all() as Array<{ content: string }>;
    messages = messageRows.map((r) => JSON.parse(r.content) as AgentMessage);
  }

  // Reconcile: prefer session DB meta, fall back to global index.
  const title = meta?.title ?? indexRow?.title ?? "New Session";
  const cwd = meta?.cwd ?? indexRow?.cwd ?? process.cwd();
  const resolvedProjectId = meta?.project_id ?? projectId ?? indexRow?.project_id ?? "default";
  const modelId = meta?.model_id ?? indexRow?.model_id ?? "gemini-2.5-pro";
  const provider = meta?.provider ?? indexRow?.provider ?? "antigravity";
  const createdAt = meta?.created_at ?? indexRow?.created_at ?? Date.now();
  const updatedAt = meta?.updated_at ?? indexRow?.updated_at ?? createdAt;

  // Repair the global index if the session DB exists but the index row is missing.
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
  options?: { cwd?: string; projectId?: string; limit?: number },
): SessionHeader[] {
  const limit = options?.limit ?? 100;

  let rows: any[];
  if (options?.cwd) {
    rows = globalDb
      .prepare(`SELECT * FROM sessions WHERE cwd = ? ORDER BY updated_at DESC LIMIT ?`)
      .all(options.cwd, limit);
  } else if (options?.projectId) {
    rows = globalDb
      .prepare(`SELECT * FROM sessions WHERE project_id = ? ORDER BY updated_at DESC LIMIT ?`)
      .all(options.projectId, limit);
  } else {
    rows = globalDb
      .prepare(`SELECT * FROM sessions ORDER BY updated_at DESC LIMIT ?`)
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
  }));
}

export function deleteSession(state: StorageState, sessionId: string): boolean {
  const { globalDb, storageDir } = state;
  const projectId = getProjectIdBySessionId(globalDb, sessionId);

  // 1. Close cached connection.
  const db = state.sessionDbs.get(sessionId);
  if (db) {
    try {
      db.close();
    } catch {
      // Ignored
    }
    state.sessionDbs.delete(sessionId);
  }

  // 2. Delete the per-session DB file.
  if (projectId) {
    removeDbFile(getSessionDbPath(storageDir, projectId, sessionId));
  } else {
    const dbPath = findSessionDbPath(storageDir, sessionId);
    if (dbPath) removeDbFile(dbPath);
  }

  // 3. Remove from the global index.
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
