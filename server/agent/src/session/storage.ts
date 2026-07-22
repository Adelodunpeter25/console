/**
 * SQLite-backed Session & History Storage Engine (WAL mode).
 * Inspired by oh-my-pi's session-persistence.ts & sql-session-storage.ts.
 *
 * Provides zero-latency synchronous SQLite storage for agent session metadata
 * and message history with transaction batching, content safety truncation, and deduplication.
 */
import DatabaseConstructor, { type Database as DatabaseType } from "better-sqlite3";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { AgentMessage, SessionHeader } from "../types/index.js";
import { initSessionDatabase } from "./schema.js";

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

export interface SessionStorageConfig {
  dbPath?: string;
}

export class SqliteSessionStorage {
  private db: DatabaseType;

  constructor(config?: SessionStorageConfig) {
    const defaultDir = path.join(process.env.HOME || process.env.USERPROFILE || ".", ".console");
    const dbPath = config?.dbPath ?? path.join(defaultDir, "console-sessions.db");

    if (dbPath !== ":memory:") {
      const dir = path.dirname(dbPath);
      try {
        fs.mkdirSync(dir, { recursive: true });
      } catch {
        // Ignored if sync directory creation fails or exists
      }
    }

    this.db = new DatabaseConstructor(dbPath);
    initSessionDatabase(this.db);
  }

  /**
   * Create a new session record.
   */
  createSession(options: {
    id?: string;
    title?: string;
    cwd: string;
    modelId: string;
    provider: string;
  }): SessionHeader {
    const id = options.id ?? crypto.randomUUID();
    const now = Date.now();
    const title = options.title?.trim() || "New Session";

    const stmt = this.db.prepare(`
      INSERT INTO sessions (id, title, cwd, model_id, provider, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?)
    `);

    stmt.run(id, title, options.cwd, options.modelId, options.provider, now, now);

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
   * Append a single message to session history (transactional).
   */
  appendMessage(sessionId: string, message: AgentMessage): void {
    const now = Date.now();
    const safeMsg = truncateForPersistence(message);
    const msgId = (safeMsg as any).id || crypto.randomUUID();
    const contentJson = JSON.stringify(safeMsg);

    const insertMsg = this.db.prepare(`
      INSERT INTO messages (id, session_id, role, content, created_at)
      VALUES (?, ?, ?, ?, ?)
    `);

    const updateSession = this.db.prepare(`
      UPDATE sessions SET updated_at = ? WHERE id = ?
    `);

    const transaction = this.db.transaction(() => {
      insertMsg.run(msgId, sessionId, safeMsg.role, contentJson, now);
      updateSession.run(now, sessionId);
    });

    transaction();
  }

  /**
   * Append multiple messages in a single atomic SQLite transaction with deduplication.
   */
  appendMessages(sessionId: string, messages: AgentMessage[]): void {
    if (messages.length === 0) return;
    const now = Date.now();

    const insertMsg = this.db.prepare(`
      INSERT OR IGNORE INTO messages (id, session_id, role, content, created_at)
      VALUES (?, ?, ?, ?, ?)
    `);

    const updateSession = this.db.prepare(`
      UPDATE sessions SET updated_at = ? WHERE id = ?
    `);

    const transaction = this.db.transaction(() => {
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
    const sessionRow = this.db.prepare(`SELECT * FROM sessions WHERE id = ?`).get(sessionId) as
      | {
          id: string;
          title: string;
          cwd: string;
          model_id: string;
          provider: string;
          created_at: number;
          updated_at: number;
        }
      | undefined;

    if (!sessionRow) return null;

    const messageRows = this.db
      .prepare(
        `SELECT content FROM messages WHERE session_id = ? ORDER BY created_at ASC, rowid ASC`,
      )
      .all(sessionId) as Array<{ content: string }>;

    const messages: AgentMessage[] = messageRows.map((r) => JSON.parse(r.content) as AgentMessage);

    const header: SessionHeader = {
      id: sessionRow.id,
      title: sessionRow.title,
      cwd: sessionRow.cwd,
      modelId: sessionRow.model_id,
      provider: sessionRow.provider,
      createdAt: sessionRow.created_at,
      updatedAt: sessionRow.updated_at,
      messageCount: messages.length,
    };

    return { header, messages };
  }

  /**
   * List saved sessions (optionally filtered by cwd).
   */
  listSessions(options?: { cwd?: string; limit?: number }): SessionHeader[] {
    const limit = options?.limit ?? 100;
    let rows: Array<{
      id: string;
      title: string;
      cwd: string;
      model_id: string;
      provider: string;
      created_at: number;
      updated_at: number;
      msg_count: number;
    }>;

    if (options?.cwd) {
      const stmt = this.db.prepare(`
        SELECT s.*, COUNT(m.id) as msg_count
        FROM sessions s
        LEFT JOIN messages m ON s.id = m.session_id
        WHERE s.cwd = ?
        GROUP BY s.id
        ORDER BY s.updated_at DESC
        LIMIT ?
      `);
      rows = stmt.all(options.cwd, limit) as typeof rows;
    } else {
      const stmt = this.db.prepare(`
        SELECT s.*, COUNT(m.id) as msg_count
        FROM sessions s
        LEFT JOIN messages m ON s.id = m.session_id
        GROUP BY s.id
        ORDER BY s.updated_at DESC
        LIMIT ?
      `);
      rows = stmt.all(limit) as typeof rows;
    }

    return rows.map((r) => ({
      id: r.id,
      title: r.title,
      cwd: r.cwd,
      modelId: r.model_id,
      provider: r.provider,
      createdAt: r.created_at,
      updatedAt: r.updated_at,
      messageCount: r.msg_count,
    }));
  }

  /**
   * Delete a session and its associated messages.
   */
  deleteSession(sessionId: string): boolean {
    const info = this.db.prepare(`DELETE FROM sessions WHERE id = ?`).run(sessionId);
    return info.changes > 0;
  }

  /**
   * Update the title of a session.
   */
  updateTitle(sessionId: string, title: string): boolean {
    const now = Date.now();
    const info = this.db
      .prepare(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`)
      .run(title.trim(), now, sessionId);
    return info.changes > 0;
  }

  /**
   * Update the active model of a session.
   */
  updateModel(sessionId: string, modelId: string, provider: string): boolean {
    const now = Date.now();
    const info = this.db
      .prepare(`UPDATE sessions SET model_id = ?, provider = ?, updated_at = ? WHERE id = ?`)
      .run(modelId, provider, now, sessionId);
    return info.changes > 0;
  }

  /**
   * Clear all sessions and messages (used for testing or reset).
   */
  clearAll(): void {
    this.db.exec(`DELETE FROM messages; DELETE FROM sessions;`);
  }

  /**
   * Close the underlying SQLite database connection.
   */
  close(): void {
    this.db.close();
  }
}
