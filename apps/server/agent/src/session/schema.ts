/**
 * SQLite Schema & Initialization for Session Storage.
 */
import type { Database } from "bun:sqlite";

export function initGlobalDatabase(db: Database): void {
  db.exec("PRAGMA journal_mode = WAL");
  db.exec("PRAGMA foreign_keys = ON");

  db.exec(`
    CREATE TABLE IF NOT EXISTS projects (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      dir TEXT NOT NULL UNIQUE,
      created_at INTEGER NOT NULL,
      updated_at INTEGER NOT NULL
    );

    -- Per-session index. Each session's full history lives in its own
    -- SQLite file (see getSessionDbPath); this table mirrors the header
    -- for fast listing without opening every session DB.
    CREATE TABLE IF NOT EXISTS sessions (
      id TEXT PRIMARY KEY,
      title TEXT NOT NULL,
      cwd TEXT NOT NULL,
      project_id TEXT,
      model_id TEXT NOT NULL,
      provider TEXT NOT NULL,
      message_count INTEGER NOT NULL DEFAULT 0,
      status TEXT NOT NULL DEFAULT 'idle',
      approval_mode TEXT NOT NULL DEFAULT 'always-ask',
      created_at INTEGER NOT NULL,
      updated_at INTEGER NOT NULL,
      deleted_at INTEGER
    );

    CREATE INDEX IF NOT EXISTS idx_projects_dir ON projects(dir);
    CREATE INDEX IF NOT EXISTS idx_sessions_cwd ON sessions(cwd);
    CREATE INDEX IF NOT EXISTS idx_sessions_project_id ON sessions(project_id);
    CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);

    CREATE TABLE IF NOT EXISTS model_favorites (
      provider TEXT NOT NULL,
      model_id TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      PRIMARY KEY (provider, model_id)
    );
  `);

  // Migration: add columns to pre-existing sessions tables.
  const cols = db.prepare("PRAGMA table_info(sessions)").all() as { name: string }[];
  if (!cols.some((c) => c.name === "status")) {
    db.exec("ALTER TABLE sessions ADD COLUMN status TEXT NOT NULL DEFAULT 'idle'");
  }
  if (!cols.some((c) => c.name === "approval_mode")) {
    db.exec("ALTER TABLE sessions ADD COLUMN approval_mode TEXT NOT NULL DEFAULT 'always-ask'");
  }
  if (!cols.some((c) => c.name === "deleted_at")) {
    db.exec("ALTER TABLE sessions ADD COLUMN deleted_at INTEGER");
  }
}

/**
 * Schema for a single session's SQLite database.
 * One row in `session_meta` holds the header; `messages` holds history.
 */
export function initSessionDatabase(db: Database): void {
  db.exec("PRAGMA journal_mode = WAL");
  db.exec("PRAGMA foreign_keys = ON");

  db.exec(`
    CREATE TABLE IF NOT EXISTS session_meta (
      id INTEGER PRIMARY KEY CHECK (id = 1),
      title TEXT NOT NULL,
      cwd TEXT NOT NULL,
      project_id TEXT,
      model_id TEXT NOT NULL,
      provider TEXT NOT NULL,
      approval_mode TEXT NOT NULL DEFAULT 'always-ask',
      repaired INTEGER NOT NULL DEFAULT 0,
      created_at INTEGER NOT NULL,
      updated_at INTEGER NOT NULL
    );

    CREATE TABLE IF NOT EXISTS messages (
      id TEXT PRIMARY KEY,
      role TEXT NOT NULL,
      content TEXT NOT NULL,
      created_at INTEGER NOT NULL
    );

    CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);

    CREATE TABLE IF NOT EXISTS session_file_changes (
      path TEXT PRIMARY KEY,
      status TEXT NOT NULL,
      additions INTEGER NOT NULL DEFAULT 0,
      deletions INTEGER NOT NULL DEFAULT 0,
      turn_index INTEGER NOT NULL DEFAULT 0,
      updated_at INTEGER NOT NULL
    );
  `);

  // Migration: add repair state to pre-existing per-session databases.
  const cols = db.prepare("PRAGMA table_info(session_meta)").all() as { name: string }[];
  if (!cols.some((c) => c.name === "repaired")) {
    db.exec("ALTER TABLE session_meta ADD COLUMN repaired INTEGER NOT NULL DEFAULT 0");
  }
}

export function initProjectDatabase(db: Database): void {
  db.exec("PRAGMA journal_mode = WAL");
  db.exec("PRAGMA foreign_keys = ON");

  db.exec(`
    CREATE TABLE IF NOT EXISTS sessions (
      id TEXT PRIMARY KEY,
      title TEXT NOT NULL,
      cwd TEXT NOT NULL,
      project_id TEXT,
      model_id TEXT NOT NULL,
      provider TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      updated_at INTEGER NOT NULL
    );

    CREATE TABLE IF NOT EXISTS messages (
      id TEXT PRIMARY KEY,
      session_id TEXT NOT NULL,
      role TEXT NOT NULL,
      content TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
    );

    CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
    CREATE INDEX IF NOT EXISTS idx_sessions_cwd ON sessions(cwd);
    CREATE INDEX IF NOT EXISTS idx_sessions_project_id ON sessions(project_id);
    CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);
  `);
}
