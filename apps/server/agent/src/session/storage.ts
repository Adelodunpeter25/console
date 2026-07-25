/**
 * SQLite-backed Session & History Storage Engine (WAL mode).
 *
 * Per-session storage layout, linked to projects on disk:
 * - A global database (`console-global.db`) holds the `projects` table and a
 *   `sessions` index mirroring session headers for fast listing.
 * - Each session is fully self-contained in its own SQLite file at
 *   `<storage>/projects/<projectId>/sessions/<sessionId>.db`
 *   (`session_meta` + `messages`). The selected model and provider are
 *   persisted there and reloaded on `loadSession`. The file location records
 *   project ownership, so a project's sessions can be enumerated/bulk-deleted
 *   by removing its directory.
 */
import DatabaseConstructor, { type Database as DatabaseType } from "better-sqlite3";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { AgentMessage, SessionHeader, ProjectInfo } from "../types/index.js";
import { initGlobalDatabase, initSessionDatabase } from "./schema.js";
import { getGlobalDbPath, getConsoleStorageDir } from "./apppaths.js";

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

interface SessionIndexRow {
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

export class SqliteSessionStorage {
  private globalDb: DatabaseType;
  private sessionDbs = new Map<string, DatabaseType>();
  private storageDir: string;

  /**
   * @param options Optional overrides for testability.
   *   - `dbPath`: global DB path (use `":memory:"` for in-memory tests).
   *   - `storageDir`: root storage directory. Per-project session DBs are
   *     written under `<storageDir>/projects/<projectId>/sessions/`. Defaults
   *     to the Console storage dir; when `dbPath` is `:memory:`, a temp dir
   *     is used so per-session files don't collide with real storage.
   */
  constructor(options?: { dbPath?: string; storageDir?: string }) {
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

    if (options?.storageDir) {
      this.storageDir = options.storageDir;
    } else if (options?.dbPath === ":memory:") {
      this.storageDir = fs.mkdtempSync(path.join(os.tmpdir(), "console-storage-"));
    } else {
      this.storageDir = getConsoleStorageDir();
    }
  }

  // ----------------------------------------------------------------------
  // Path helpers (project-scoped)
  // ----------------------------------------------------------------------

  private getProjectStorageDir(projectId: string): string {
    return path.join(this.storageDir, "projects", projectId);
  }

  private getProjectSessionsDir(projectId: string): string {
    return path.join(this.getProjectStorageDir(projectId), "sessions");
  }

  private getSessionDbPath(projectId: string, sessionId: string): string {
    return path.join(this.getProjectSessionsDir(projectId), `${sessionId}.db`);
  }

  private ensureDir(filePath: string): void {
    try {
      fs.mkdirSync(path.dirname(filePath), { recursive: true });
    } catch {
      // Ignored if folder exists
    }
  }

  /**
   * Resolves the project ID for a session from the global index.
   */
  private getProjectIdBySessionId(sessionId: string): string | null {
    const row = this.globalDb
      .prepare("SELECT project_id FROM sessions WHERE id = ?")
      .get(sessionId) as { project_id: string } | undefined;
    return row ? row.project_id : null;
  }

  /**
   * Opens (and caches) the SQLite connection for a single session.
   */
  private getSessionDb(sessionId: string, projectId: string): DatabaseType {
    let db = this.sessionDbs.get(sessionId);
    if (!db) {
      const dbPath = this.getSessionDbPath(projectId, sessionId);
      this.ensureDir(dbPath);
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
   * Removes every session DB file under the project's sessions directory.
   */
  deleteProject(projectId: string): boolean {
    // 1. Close and remove every session DB owned by this project.
    const sessionIds = this.globalDb
      .prepare(`SELECT id FROM sessions WHERE project_id = ?`)
      .all(projectId) as Array<{ id: string }>;
    for (const { id } of sessionIds) {
      const db = this.sessionDbs.get(id);
      if (db) {
        try {
          db.close();
        } catch {
          // Ignored
        }
        this.sessionDbs.delete(id);
      }
    }

    // 2. Delete the project's entire storage directory (sessions + DBs).
    try {
      const projDir = this.getProjectStorageDir(projectId);
      if (fs.existsSync(projDir)) {
        fs.rmSync(projDir, { recursive: true, force: true });
      }
    } catch {
      // Ignored
    }

    // 3. Clear the global index rows.
    this.globalDb.prepare(`DELETE FROM sessions WHERE project_id = ?`).run(projectId);
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
    const sessionDb = this.getSessionDb(id, projectId);
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
    const projectId = this.getProjectIdBySessionId(sessionId);
    if (!projectId) return;

    const now = Date.now();
    const safeMsg = truncateForPersistence(message);
    const msgId = (safeMsg as any).id || crypto.randomUUID();
    const contentJson = JSON.stringify(safeMsg);

    const sessionDb = this.getSessionDb(sessionId, projectId);
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
    const projectId = this.getProjectIdBySessionId(sessionId);
    if (!projectId) return;

    const now = Date.now();
    const sessionDb = this.getSessionDb(sessionId, projectId);
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
      .get(sessionId) as SessionIndexRow | undefined;

    // Resolve the project ID for routing. Prefer the index; if missing, scan
    // the project directories for an orphaned session DB file.
    let projectId: string | null = indexRow?.project_id ?? null;
    let dbPath: string | undefined;

    if (projectId) {
      dbPath = this.getSessionDbPath(projectId, sessionId);
    } else {
      dbPath = this.findSessionDbPath(sessionId);
      if (dbPath) {
        // Derive projectId from the discovered path.
        const sessionsIdx = dbPath.indexOf(`${path.sep}sessions${path.sep}`);
        if (sessionsIdx > 0) {
          const projPart = dbPath.slice(0, sessionsIdx);
          projectId = path.basename(projPart);
        }
      }
    }

    const hasDbFile = !!dbPath && fs.existsSync(dbPath);
    if (!indexRow && !hasDbFile) return null;

    // Authoritative meta comes from the session DB when present.
    let meta: SessionMetaRow | null = null;
    let messages: AgentMessage[] = [];

    if (hasDbFile && projectId) {
      const sessionDb = this.getSessionDb(sessionId, projectId);
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

    // If the global index was missing but the session DB exists, repair it.
    if (!indexRow) {
      this.globalDb
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
        .prepare(`SELECT * FROM sessions WHERE cwd = ? ORDER BY updated_at DESC LIMIT ?`)
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
    const projectId = this.getProjectIdBySessionId(sessionId);

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
    if (projectId) {
      const dbPath = this.getSessionDbPath(projectId, sessionId);
      this.removeDbFile(dbPath);
    } else {
      // Orphan: scan for the file.
      const dbPath = this.findSessionDbPath(sessionId);
      if (dbPath) this.removeDbFile(dbPath);
    }

    // 3. Remove from the global index.
    const info = this.globalDb.prepare(`DELETE FROM sessions WHERE id = ?`).run(sessionId);
    return info.changes > 0;
  }

  /**
   * Update the title of a session (both session DB and global index).
   */
  updateTitle(sessionId: string, title: string): boolean {
    const projectId = this.getProjectIdBySessionId(sessionId);
    const now = Date.now();
    const trimmed = title.trim();

    if (projectId) {
      const dbPath = this.getSessionDbPath(projectId, sessionId);
      if (this.sessionDbs.has(sessionId) || fs.existsSync(dbPath)) {
        const sessionDb = this.getSessionDb(sessionId, projectId);
        sessionDb
          .prepare(`UPDATE session_meta SET title = ?, updated_at = ? WHERE id = 1`)
          .run(trimmed, now);
      }
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
    const projectId = this.getProjectIdBySessionId(sessionId);
    const now = Date.now();

    if (projectId) {
      const dbPath = this.getSessionDbPath(projectId, sessionId);
      if (this.sessionDbs.has(sessionId) || fs.existsSync(dbPath)) {
        const sessionDb = this.getSessionDb(sessionId, projectId);
        sessionDb
          .prepare(
            `UPDATE session_meta SET model_id = ?, provider = ?, updated_at = ? WHERE id = 1`,
          )
          .run(modelId, provider, now);
      }
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
   * Remove a `.db` file and its WAL/SHM sidecars.
   */
  private removeDbFile(dbPath: string): void {
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
  private findSessionDbPath(sessionId: string): string | undefined {
    const projectsRoot = path.join(this.storageDir, "projects");
    let projects: string[] = [];
    try {
      projects = fs.readdirSync(projectsRoot, { withFileTypes: true })
        .filter((d) => d.isDirectory())
        .map((d) => d.name);
    } catch {
      // No projects directory yet.
    }

    for (const projId of projects) {
      const candidate = this.getSessionDbPath(projId, sessionId);
      if (fs.existsSync(candidate)) return candidate;
    }
    return undefined;
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
