/**
 * SQLite-backed Session & History Storage Engine (WAL mode).
 *
 * Per-session storage layout:
 * - A global database (`console-global.db`) holds the `projects` table and a
 *   `sessions` index mirroring session headers for fast listing.
 * - Each session is fully self-contained in its own SQLite file at
 *   `sessions/<session-id>.db` (`session_meta` + `messages`). The selected
 *   model and provider are persisted there and reloaded on `loadSession`.
 */
import DatabaseConstructor, { type Database as DatabaseType } from "better-sqlite3";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { AgentMessage, SessionHeader, ProjectInfo } from "../types/index.js";
import { initGlobalDatabase, initSessionDatabase } from "./schema.js";
import { getGlobalDbPath, getSessionsDir as defaultGetSessionsDir } from "./apppaths.js";

const MAX_PERSIST_CHARS = 500_000;
const TRUNCATION_NOTICE = "\n\n[Session persistence truncated large content]";

/**
 * Truncate strings in message content if they exceed safety limits (500k chars).
 */
function truncateForPersistence(message: AgentMessage): AgentMessage {
  if (message.role === "toolResult") {
    const truncatedResults = message.results.map((res) => {
      if (typeof res.content === "string" && res.content.length > MAX_PERSIST_CHARS) {
        return {
          ...res,
          content: res.content.slice(0, MAX_PERSIST_CHARS) + TRUNCATION_NOTICE,
        };
      }
      return res;
    });
    return { ...message, results: truncatedResults };
  }
  return message;
}

interface SessionMetaRow {
  title: string;
  cwd: string;
  project_id: string | null;
  model_id: string;
  provider: string;
  created_at: number;
  updated_at: number;
}

export class SqliteSessionStorage {
  private globalDb: DatabaseType;
  private sessionDbs = new Map<string, DatabaseType>();
  private sessionsDir: string;

  /**
   * @param options Optional overrides for testability.
   *   - `dbPath`: global DB path (use `":memory:"` for in-memory tests).
   *   - `sessionsDir`: directory for per-session DB files. Defaults to the
   *     Console storage dir; when `dbPath` is `:memory:`, a temp dir is used
   *     so per-session files don't collide with real storage.
   */
  constructor(options?: { dbPath?: string; sessionsDir?: string }) {
    const globalDbPath = options?.dbPath ?? getGlobalDbPath();
    const dir = path.dirname(globalDbPath);
    if (dir !== "." && dir !== "/") {
      try {
        fs.mkdirSync(dir, { recursive: true });
      } catch {
        // Ignored if folder exists
      }
    }

    this.globalDb = new DatabaseConstructor(globalDbPath);
    initGlobalDatabase(this.globalDb);

    if (options?.sessionsDir) {
      this.sessionsDir = options.sessionsDir;
    } else if (options?.dbPath === ":memory:") {
      this.sessionsDir = fs.mkdtempSync(path.join(os.tmpdir(), "console-sessions-"));
    } else {
      this.sessionsDir = defaultGetSessionsDir();
    }
    try {
      fs.mkdirSync(this.sessionsDir, { recursive: true });
    } catch {
      // Ignored if folder exists
    }
  }

  /**
   * Resolves the SQLite path for a single session.
   */
  private getSessionDbPath(sessionId: string): string {
    return path.join(this.sessionsDir, `${sessionId}.db`);
  }

  /**
   * Opens (and caches) the SQLite connection for a single session.
   */
  private getSessionDb(sessionId: string): DatabaseType {
    let db = this.sessionDbs.get(sessionId);
    if (!db) {
      const dbPath = this.getSessionDbPath(sessionId);
      const dir = path.dirname(dbPath);
      try {
        fs.mkdirSync(dir, { recursive: true });
      } catch {
        // Ignored if folder exists
      }
      db = new DatabaseConstructor(dbPath);
      initSessionDatabase(db);
      this.sessionDbs.set(sessionId, db);
    }
    return db;
  }

  // ----------------------------------------------------------------------
  // Projects (global DB only)
  // ----------------------------------------------------------------------

  /**
   * Create a new project record.
   */
  createProject(options: { id?: string; name: string; dir: string }): ProjectInfo {
    const id = options.id ?? crypto.randomUUID();
    const now = Date.now();
    const stmt = this.globalDb.prepare(`
      INSERT INTO projects (id, name, dir, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?)
      ON CONFLICT(dir) DO UPDATE SET
        name = excluded.name,
        updated_at = excluded.updated_at
    `);
    stmt.run(id, options.name, options.dir, now, now);

    const row = this.globalDb
      .prepare(`SELECT * FROM projects WHERE dir = ?`)
      .get(options.dir) as any;
    return {
      id: row.id,
      name: row.name,
      path: row.dir,
      createdAt: row.created_at,
      updatedAt: row.updated_at,
    };
  }

  /**
   * Get a project by ID.
   */
  getProject(projectId: string): ProjectInfo | null {
    const row = this.globalDb.prepare(`SELECT * FROM projects WHERE id = ?`).get(projectId) as any;
    if (!row) return null;
    return {
      id: row.id,
      name: row.name,
      path: row.dir,
      createdAt: row.created_at,
      updatedAt: row.updated_at,
    };
  }

  /**
   * Get a project by its directory path.
   */
  getProjectByDir(dir: string): ProjectInfo | null {
    const row = this.globalDb.prepare(`SELECT * FROM projects WHERE dir = ?`).get(dir) as any;
    if (!row) return null;
    return {
      id: row.id,
      name: row.name,
      path: row.dir,
      createdAt: row.created_at,
      updatedAt: row.updated_at,
    };
  }

  /**
   * List all projects.
   */
  listProjects(): ProjectInfo[] {
    const rows = this.globalDb
      .prepare(`SELECT * FROM projects ORDER BY updated_at DESC`)
      .all() as any[];
    return rows.map((row) => ({
      id: row.id,
      name: row.name,
      path: row.dir,
      createdAt: row.created_at,
      updatedAt: row.updated_at,
    }));
  }

  /**
   * Delete a project and all sessions belonging to it.
   */
  deleteProject(projectId: string): boolean {
    // 1. Delete every session DB file owned by this project.
    const sessionIds = this.globalDb
      .prepare(`SELECT id FROM sessions WHERE project_id = ?`)
      .all(projectId) as Array<{ id: string }>;
    for (const { id } of sessionIds) {
      this.deleteSession(id);
    }

    // 2. Delete the project entry.
    const info = this.globalDb.prepare(`DELETE FROM projects WHERE id = ?`).run(projectId);
    return info.changes > 0;
  }

  // ----------------------------------------------------------------------
  // Sessions (per-session DB + global index)
  // ----------------------------------------------------------------------

  /**
   * Create a new session in its own SQLite file and index it globally.
   */
  createSession(options: {
    id?: string;
    title?: string;
    cwd: string;
    projectId?: string;
    modelId: string;
    provider: string;
  }): SessionHeader {
    const id = options.id ?? crypto.randomUUID();
    const now = Date.now();
    const title = options.title?.trim() || "New Session";
    const projectId = options.projectId || "default";

    // 1. Write header to the global index (fast listing).
    this.globalDb
      .prepare(
        `INSERT INTO sessions
          (id, title, cwd, project_id, model_id, provider, message_count, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
      )
      .run(id, title, options.cwd, projectId, options.modelId, options.provider, now, now);

    // 2. Initialize the per-session DB and persist the authoritative meta row.
    const sessionDb = this.getSessionDb(id);
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
    };
  }

  /**
   * Append a single message to session history.
   */
  appendMessage(sessionId: string, message: AgentMessage): void {
    const now = Date.now();
    const safeMsg = truncateForPersistence(message);
    const msgId = (safeMsg as any).id || crypto.randomUUID();
    const contentJson = JSON.stringify(safeMsg);

    const sessionDb = this.getSessionDb(sessionId);
    const insertMsg = sessionDb.prepare(
      `INSERT INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`,
    );
    const tx = sessionDb.transaction(() => {
      insertMsg.run(msgId, safeMsg.role, contentJson, now);
    });
    tx();

    this.bumpSessionUpdated(sessionId, now, 1);
  }

  /**
   * Append multiple messages in a transaction.
   */
  appendMessages(sessionId: string, messages: AgentMessage[]): void {
    if (messages.length === 0) return;
    const now = Date.now();
    const sessionDb = this.getSessionDb(sessionId);
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

    this.bumpSessionUpdated(sessionId, now, inserted);
  }

  /**
   * Load session header and all associated messages from the per-session DB.
   * Falls back to the global index header if the session DB meta is missing.
   */
  loadSession(sessionId: string): { header: SessionHeader; messages: AgentMessage[] } | null {
    const indexRow = this.globalDb
      .prepare(`SELECT * FROM sessions WHERE id = ?`)
      .get(sessionId) as
      | {
          id: string;
          title: string;
          cwd: string;
          project_id: string | null;
          model_id: string;
          provider: string;
          message_count: number;
          created_at: number;
          updated_at: number;
        }
      | undefined;

    // No index row and no session DB file => session does not exist.
    const dbPath = this.getSessionDbPath(sessionId);
    const hasDbFile = fs.existsSync(dbPath);
    if (!indexRow && !hasDbFile) return null;

    // Authoritative meta comes from the session DB when present.
    let meta: SessionMetaRow | null = null;
    let messages: AgentMessage[] = [];

    if (hasDbFile) {
      const sessionDb = this.getSessionDb(sessionId);
      meta = (sessionDb
        .prepare(`SELECT title, cwd, project_id, model_id, provider, created_at, updated_at FROM session_meta WHERE id = 1`)
        .get() as SessionMetaRow | undefined) ?? null;

      const messageRows = sessionDb
        .prepare(`SELECT content FROM messages ORDER BY created_at ASC, rowid ASC`)
        .all() as Array<{ content: string }>;
      messages = messageRows.map((r) => JSON.parse(r.content) as AgentMessage);
    }

    // Reconcile: prefer session DB meta, fall back to global index.
    const title = meta?.title ?? indexRow?.title ?? "New Session";
    const cwd = meta?.cwd ?? indexRow?.cwd ?? process.cwd();
    const projectId = meta?.project_id ?? indexRow?.project_id ?? undefined;
    const modelId = meta?.model_id ?? indexRow?.model_id ?? "gemini-2.5-pro";
    const provider = meta?.provider ?? indexRow?.provider ?? "antigravity";
    const createdAt = meta?.created_at ?? indexRow?.created_at ?? Date.now();
    const updatedAt = meta?.updated_at ?? indexRow?.updated_at ?? createdAt;

    // If the global index was missing but the session DB exists, repair it.
    if (!indexRow) {
      this.globalDb
        .prepare(
          `INSERT INTO sessions
            (id, title, cwd, project_id, model_id, provider, message_count, created_at, updated_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        )
        .run(sessionId, title, cwd, projectId ?? null, modelId, provider, messages.length, createdAt, updatedAt);
    }

    const header: SessionHeader = {
      id: sessionId,
      title,
      cwd,
      modelId,
      provider,
      createdAt,
      updatedAt,
      messageCount: messages.length,
    };

    return { header, messages };
  }

  /**
   * List saved sessions directly from the global index (no per-DB opens).
   */
  listSessions(options?: { cwd?: string; projectId?: string; limit?: number }): SessionHeader[] {
    const limit = options?.limit ?? 100;

    let rows: any[];
    if (options?.cwd) {
      rows = this.globalDb
        .prepare(
          `SELECT * FROM sessions WHERE cwd = ? ORDER BY updated_at DESC LIMIT ?`,
        )
        .all(options.cwd, limit);
    } else if (options?.projectId) {
      rows = this.globalDb
        .prepare(
          `SELECT * FROM sessions WHERE project_id = ? ORDER BY updated_at DESC LIMIT ?`,
        )
        .all(options.projectId, limit);
    } else {
      rows = this.globalDb
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
    }));
  }

  /**
   * Delete a session: close its DB connection, remove the file, drop the index row.
   */
  deleteSession(sessionId: string): boolean {
    // 1. Close and discard any cached connection.
    const db = this.sessionDbs.get(sessionId);
    if (db) {
      try {
        db.close();
      } catch {
        // Ignored
      }
      this.sessionDbs.delete(sessionId);
    }

    // 2. Delete the per-session DB file (and WAL/SHM sidecars).
    try {
      const dbPath = this.getSessionDbPath(sessionId);
      if (fs.existsSync(dbPath)) {
        fs.unlinkSync(dbPath);
      }
      for (const ext of ["-wal", "-shm"]) {
        const sidecar = `${dbPath}${ext}`;
        if (fs.existsSync(sidecar)) fs.unlinkSync(sidecar);
      }
    } catch {
      // Ignored
    }

    // 3. Remove from the global index.
    const info = this.globalDb.prepare(`DELETE FROM sessions WHERE id = ?`).run(sessionId);
    return info.changes > 0;
  }

  /**
   * Update the title of a session (both session DB and global index).
   */
  updateTitle(sessionId: string, title: string): boolean {
    const now = Date.now();
    const trimmed = title.trim();

    if (this.sessionDbs.has(sessionId) || fs.existsSync(this.getSessionDbPath(sessionId))) {
      const sessionDb = this.getSessionDb(sessionId);
      sessionDb
        .prepare(`UPDATE session_meta SET title = ?, updated_at = ? WHERE id = 1`)
        .run(trimmed, now);
    }

    const info = this.globalDb
      .prepare(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`)
      .run(trimmed, now, sessionId);
    return info.changes > 0;
  }

  /**
   * Update the active model + provider of a session (both session DB and global index).
   */
  updateModel(sessionId: string, modelId: string, provider: string): boolean {
    const now = Date.now();

    if (this.sessionDbs.has(sessionId) || fs.existsSync(this.getSessionDbPath(sessionId))) {
      const sessionDb = this.getSessionDb(sessionId);
      sessionDb
        .prepare(`UPDATE session_meta SET model_id = ?, provider = ?, updated_at = ? WHERE id = 1`)
        .run(modelId, provider, now);
    }

    const info = this.globalDb
      .prepare(`UPDATE sessions SET model_id = ?, provider = ?, updated_at = ? WHERE id = ?`)
      .run(modelId, provider, now, sessionId);
    return info.changes > 0;
  }

  /**
   * Refresh the global index's `updated_at` and `message_count` for a session.
   */
  private bumpSessionUpdated(sessionId: string, now: number, added: number): void {
    this.globalDb
      .prepare(
        `UPDATE sessions SET updated_at = ?, message_count = message_count + ? WHERE id = ?`,
      )
      .run(now, added, sessionId);
  }

  /**
   * Clear all sessions and messages (used for testing or reset).
   */
  clearAll(): void {
    this.globalDb.exec(`DELETE FROM sessions; DELETE FROM projects;`);
    for (const [, db] of this.sessionDbs) {
      try {
        db.exec(`DELETE FROM messages; DELETE FROM session_meta;`);
      } catch {
        // Ignored
      }
    }
  }

  /**
   * Close the database connections.
   */
  close(): void {
    this.globalDb.close();
    for (const [, db] of this.sessionDbs) {
      try {
        db.close();
      } catch {
        // Ignored
      }
    }
    this.sessionDbs.clear();
  }
}
