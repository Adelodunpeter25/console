/**
 * SQLite schema for a memory database. The same schema backs both a
 * per-project `memory.db` and the shared root `memory-global.db` — only the
 * file path differs (see registry.ts).
 */
import type { Database } from "bun:sqlite";

export function initMemoryDatabase(db: Database): void {
  try {
    if (db.filename && db.filename !== "" && db.filename !== ":memory:") {
      db.exec("PRAGMA journal_mode = WAL");
    }
  } catch {}

  db.exec(`
    CREATE TABLE IF NOT EXISTS memories (
      id TEXT PRIMARY KEY,
      scope TEXT NOT NULL,
      content TEXT NOT NULL,
      tags TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      updated_at INTEGER NOT NULL
    );

    CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);
  `);
}
