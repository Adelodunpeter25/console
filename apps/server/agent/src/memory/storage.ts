/**
 * SQLite-backed CRUD store for a single memory database file. One instance
 * targets exactly one file — the caller (registry.ts) decides whether that
 * file is a project's `memory.db` or the shared `memory-global.db`.
 */
import { Database as DatabaseConstructor, type Database as DatabaseType } from "bun:sqlite";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { initMemoryDatabase } from "./schema.js";
import type {
  MemoryEntry,
  MemoryListFilter,
  MemoryScope,
  MemoryStoreInput,
  MemoryUpdateInput,
} from "./types.js";

interface MemoryRow {
  id: string;
  scope: string;
  content: string;
  tags: string;
  created_at: number;
  updated_at: number;
}

function rowToEntry(row: MemoryRow): MemoryEntry {
  return {
    id: row.id,
    scope: row.scope as MemoryScope,
    content: row.content,
    tags: JSON.parse(row.tags) as string[],
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export class SqliteMemoryStorage {
  private db: DatabaseType;

  constructor(dbPath: string, scope: MemoryScope) {
    this.scope = scope;
    if (dbPath !== ":memory:") {
      fs.mkdirSync(path.dirname(dbPath), { recursive: true });
    }
    this.db = new DatabaseConstructor(dbPath);
    initMemoryDatabase(this.db);
  }

  readonly scope: MemoryScope;

  store(input: MemoryStoreInput): MemoryEntry {
    const now = Date.now();
    const entry: MemoryEntry = {
      id: crypto.randomUUID(),
      scope: this.scope,
      content: input.content,
      tags: input.tags ?? [],
      createdAt: now,
      updatedAt: now,
    };
    this.db
      .prepare(
        `INSERT INTO memories (id, scope, content, tags, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?)`,
      )
      .run(entry.id, entry.scope, entry.content, JSON.stringify(entry.tags), entry.createdAt, entry.updatedAt);
    return entry;
  }

  get(id: string): MemoryEntry | null {
    const row = this.db.prepare(`SELECT * FROM memories WHERE id = ?`).get(id) as MemoryRow | undefined;
    return row ? rowToEntry(row) : null;
  }

  list(filter?: MemoryListFilter): MemoryEntry[] {
    const rows = this.db
      .prepare(`SELECT * FROM memories ORDER BY updated_at DESC`)
      .all() as MemoryRow[];
    const entries = rows.map(rowToEntry);
    if (!filter?.tags || filter.tags.length === 0) return entries;
    const wanted = new Set(filter.tags);
    return entries.filter((entry) => entry.tags.some((tag) => wanted.has(tag)));
  }

  update(id: string, patch: MemoryUpdateInput): MemoryEntry | null {
    const existing = this.get(id);
    if (!existing) return null;
    const updated: MemoryEntry = {
      ...existing,
      content: patch.content ?? existing.content,
      tags: patch.tags ?? existing.tags,
      updatedAt: Date.now(),
    };
    this.db
      .prepare(`UPDATE memories SET content = ?, tags = ?, updated_at = ? WHERE id = ?`)
      .run(updated.content, JSON.stringify(updated.tags), updated.updatedAt, id);
    return updated;
  }

  remove(id: string): boolean {
    const result = this.db.prepare(`DELETE FROM memories WHERE id = ?`).run(id);
    return result.changes > 0;
  }

  close(): void {
    this.db.close();
  }
}
