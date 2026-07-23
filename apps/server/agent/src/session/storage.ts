/**
 * SQLite-backed Session & History Storage Engine (WAL mode).
 * Provides dynamic multi-database storage routing:
 * - A global database for project configurations.
 * - Per-project databases for isolated session histories & message logs.
 */
import DatabaseConstructor, { type Database as DatabaseType } from "better-sqlite3";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { AgentMessage, SessionHeader, ProjectInfo } from "../types/index.js";
import { initGlobalDatabase, initProjectDatabase } from "./schema.js";
import { getGlobalDbPath, getProjectDbPath } from "./apppaths.js";

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

export class SqliteSessionStorage {
  private globalDb: DatabaseType;
  private projectDbs = new Map<string, DatabaseType>();

  constructor() {
    const globalDbPath = getGlobalDbPath();
    const dir = path.dirname(globalDbPath);
    try {
      fs.mkdirSync(dir, { recursive: true });
    } catch {
      // Ignored if folder exists
    }

    this.globalDb = new DatabaseConstructor(globalDbPath);
    initGlobalDatabase(this.globalDb);
  }

  /**
   * Retrieves or initializes the SQLite connection for a specific project.
   */
  private getProjectDb(projectId: string): DatabaseType {
    let db = this.projectDbs.get(projectId);
    if (!db) {
      const dbPath = getProjectDbPath(projectId);
      const dir = path.dirname(dbPath);
      try {
        fs.mkdirSync(dir, { recursive: true });
      } catch {
        // Ignored if folder exists
      }
      db = new DatabaseConstructor(dbPath);
      initProjectDatabase(db);
      this.projectDbs.set(projectId, db);
    }
    return db;
  }

  /**
   * Look up which project ID a specific session ID belongs to.
   */
  private getProjectIdBySessionId(sessionId: string): string | null {
    const row = this.globalDb
      .prepare("SELECT project_id FROM sessions_lookup WHERE id = ?")
      .get(sessionId) as { project_id: string } | undefined;
    return row ? row.project_id : null;
  }

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
   * Delete a project.
   */
  deleteProject(projectId: string): boolean {
    // 1. Delete associated database connection if open
    const db = this.projectDbs.get(projectId);
    if (db) {
      db.close();
      this.projectDbs.delete(projectId);
    }

    // 2. Delete project database file on disk if exists
    try {
      const dbPath = getProjectDbPath(projectId);
      if (fs.existsSync(dbPath)) {
        fs.unlinkSync(dbPath);
      }
    } catch {
      // Ignored
    }

    // 3. Clear sessions mapping lookup
    this.globalDb.prepare(`DELETE FROM sessions_lookup WHERE project_id = ?`).run(projectId);

    // 4. Delete the project entry
    const info = this.globalDb.prepare(`DELETE FROM projects WHERE id = ?`).run(projectId);
    return info.changes > 0;
  }

  /**
   * Create a new session record inside the appropriate project database.
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

    // 1. Write session meta to project DB
    const projectDb = this.getProjectDb(projectId);
    const stmt = projectDb.prepare(`
      INSERT INTO sessions (id, title, cwd, project_id, model_id, provider, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `);
    stmt.run(id, title, options.cwd, projectId, options.modelId, options.provider, now, now);

    // 2. Add to global session lookup index
    const lookupStmt = this.globalDb.prepare(`
      INSERT INTO sessions_lookup (id, project_id)
      VALUES (?, ?)
    `);
    lookupStmt.run(id, projectId);

    return {
      id,
      title,
      cwd: options.cwd,
      projectId,
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

    const projectDb = this.getProjectDb(projectId);
    const insertMsg = projectDb.prepare(`
      INSERT INTO messages (id, session_id, role, content, created_at)
      VALUES (?, ?, ?, ?, ?)
    `);

    const updateSession = projectDb.prepare(`
      UPDATE sessions SET updated_at = ? WHERE id = ?
    `);

    const transaction = projectDb.transaction(() => {
      insertMsg.run(msgId, sessionId, safeMsg.role, contentJson, now);
      updateSession.run(now, sessionId);
    });

    transaction();
  }

  /**
   * Append multiple messages in a transaction.
   */
  appendMessages(sessionId: string, messages: AgentMessage[]): void {
    if (messages.length === 0) return;
    const projectId = this.getProjectIdBySessionId(sessionId);
    if (!projectId) return;

    const now = Date.now();
    const projectDb = this.getProjectDb(projectId);
    const insertMsg = projectDb.prepare(`
      INSERT OR IGNORE INTO messages (id, session_id, role, content, created_at)
      VALUES (?, ?, ?, ?, ?)
    `);

    const updateSession = projectDb.prepare(`
      UPDATE sessions SET updated_at = ? WHERE id = ?
    `);

    const transaction = projectDb.transaction(() => {
      for (const msg of messages) {
        const safeMsg = truncateForPersistence(msg);
        const msgId =
          (safeMsg as any).id ||
          crypto.createHash("sha256").update(JSON.stringify(safeMsg)).digest("hex").slice(0, 32);
        insertMsg.run(msgId, sessionId, safeMsg.role, JSON.stringify(safeMsg), now);
      }
      updateSession.run(now, sessionId);
    });

    transaction();
  }

  /**
   * Load session header and all associated messages.
   */
  loadSession(sessionId: string): { header: SessionHeader; messages: AgentMessage[] } | null {
    const projectId = this.getProjectIdBySessionId(sessionId);
    if (!projectId) return null;

    const projectDb = this.getProjectDb(projectId);
    const sessionRow = projectDb.prepare(`SELECT * FROM sessions WHERE id = ?`).get(sessionId) as
      | {
          id: string;
          title: string;
          cwd: string;
          project_id: string | null;
          model_id: string;
          provider: string;
          created_at: number;
          updated_at: number;
        }
      | undefined;

    if (!sessionRow) return null;

    const messageRows = projectDb
      .prepare(
        `SELECT content FROM messages WHERE session_id = ? ORDER BY created_at ASC, rowid ASC`,
      )
      .all(sessionId) as Array<{ content: string }>;

    const messages: AgentMessage[] = messageRows.map((r) => JSON.parse(r.content) as AgentMessage);

    const header: SessionHeader = {
      id: sessionRow.id,
      title: sessionRow.title,
      cwd: sessionRow.cwd,
      projectId: sessionRow.project_id || undefined,
      modelId: sessionRow.model_id,
      provider: sessionRow.provider,
      createdAt: sessionRow.created_at,
      updatedAt: sessionRow.updated_at,
      messageCount: messages.length,
    };

    return { header, messages };
  }

  /**
   * List saved sessions.
   */
  listSessions(options?: { cwd?: string; projectId?: string; limit?: number }): SessionHeader[] {
    const limit = options?.limit ?? 100;

    const querySessionsFromDb = (
      db: DatabaseType,
      projId?: string,
      cwdVal?: string,
    ): SessionHeader[] => {
      let rows: any[];
      if (cwdVal) {
        const stmt = db.prepare(`
          SELECT s.*, COUNT(m.id) as msg_count
          FROM sessions s
          LEFT JOIN messages m ON s.id = m.session_id
          WHERE s.cwd = ?
          GROUP BY s.id
          ORDER BY s.updated_at DESC
          LIMIT ?
        `);
        rows = stmt.all(cwdVal, limit);
      } else if (projId) {
        const stmt = db.prepare(`
          SELECT s.*, COUNT(m.id) as msg_count
          FROM sessions s
          LEFT JOIN messages m ON s.id = m.session_id
          WHERE s.project_id = ?
          GROUP BY s.id
          ORDER BY s.updated_at DESC
          LIMIT ?
        `);
        rows = stmt.all(projId, limit);
      } else {
        const stmt = db.prepare(`
          SELECT s.*, COUNT(m.id) as msg_count
          FROM sessions s
          LEFT JOIN messages m ON s.id = m.session_id
          GROUP BY s.id
          ORDER BY s.updated_at DESC
          LIMIT ?
        `);
        rows = stmt.all(limit);
      }

      return rows.map((r) => ({
        id: r.id,
        title: r.title,
        cwd: r.cwd,
        projectId: r.project_id || undefined,
        modelId: r.model_id,
        provider: r.provider,
        createdAt: r.created_at,
        updatedAt: r.updated_at,
        messageCount: r.msg_count,
      }));
    };

    // 1. Query filtered by projectId
    if (options?.projectId) {
      const db = this.getProjectDb(options.projectId);
      return querySessionsFromDb(db, options.projectId);
    }

    // 2. Query filtered by cwd (resolve project first)
    if (options?.cwd) {
      const proj = this.getProjectByDir(options.cwd);
      const projectId = proj ? proj.id : "default";
      const db = this.getProjectDb(projectId);
      return querySessionsFromDb(db, projectId, options.cwd);
    }

    // 3. Unfiltered - merge from all projects
    const allProjects = this.listProjects();
    const allSessions: SessionHeader[] = [];

    // Always include default project sessions
    try {
      const defaultDb = this.getProjectDb("default");
      allSessions.push(...querySessionsFromDb(defaultDb, "default"));
    } catch {
      // Ignored
    }

    for (const proj of allProjects) {
      if (proj.id === "default") continue;
      try {
        const db = this.getProjectDb(proj.id);
        allSessions.push(...querySessionsFromDb(db, proj.id));
      } catch {
        // Ignored
      }
    }

    // Sort combined sessions
    return allSessions.sort((a, b) => b.updatedAt - a.updatedAt).slice(0, limit);
  }

  /**
   * Delete a session and its associated messages.
   */
  deleteSession(sessionId: string): boolean {
    const projectId = this.getProjectIdBySessionId(sessionId);
    if (!projectId) return false;

    const projectDb = this.getProjectDb(projectId);
    const info = projectDb.prepare(`DELETE FROM sessions WHERE id = ?`).run(sessionId);

    this.globalDb.prepare(`DELETE FROM sessions_lookup WHERE id = ?`).run(sessionId);

    return info.changes > 0;
  }

  /**
   * Update the title of a session.
   */
  updateTitle(sessionId: string, title: string): boolean {
    const projectId = this.getProjectIdBySessionId(sessionId);
    if (!projectId) return false;

    const now = Date.now();
    const projectDb = this.getProjectDb(projectId);
    const info = projectDb
      .prepare(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`)
      .run(title.trim(), now, sessionId);
    return info.changes > 0;
  }

  /**
   * Update the active model of a session.
   */
  updateModel(sessionId: string, modelId: string, provider: string): boolean {
    const projectId = this.getProjectIdBySessionId(sessionId);
    if (!projectId) return false;

    const now = Date.now();
    const projectDb = this.getProjectDb(projectId);
    const info = projectDb
      .prepare(`UPDATE sessions SET model_id = ?, provider = ?, updated_at = ? WHERE id = ?`)
      .run(modelId, provider, now, sessionId);
    return info.changes > 0;
  }

  /**
   * Clear all sessions and messages (used for testing or reset).
   */
  clearAll(): void {
    this.globalDb.exec(`DELETE FROM sessions_lookup; DELETE FROM projects;`);
    for (const [projectId, db] of this.projectDbs) {
      try {
        db.exec(`DELETE FROM messages; DELETE FROM sessions;`);
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
    for (const [projectId, db] of this.projectDbs) {
      try {
        db.close();
      } catch {
        // Ignored
      }
    }
    this.projectDbs.clear();
  }
}
