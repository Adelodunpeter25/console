/**
 * Project CRUD operations against the global database.
 *
 * Sessions belonging to a project live as SQLite files under the project's
 * storage directory; `deleteProject` removes that whole tree.
 */
import type { Database as DatabaseType } from "bun:sqlite";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { ProjectInfo } from "@/agent/src/types/index.js";
import type { StorageState } from "./utils.js";

interface CreateProjectOptions {
  id?: string;
  name: string;
  dir: string;
}

/** Insert or upsert a project row and return the normalized `ProjectInfo`. */
export function createProject(db: DatabaseType, options: CreateProjectOptions): ProjectInfo {
  const id = options.id ?? crypto.randomUUID();
  const now = Date.now();
  const stmt = db.prepare(`
    INSERT INTO projects (id, name, dir, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?)
    ON CONFLICT(dir) DO UPDATE SET
      name = excluded.name,
      updated_at = excluded.updated_at
  `);
  stmt.run(id, options.name, options.dir, now, now);

  const row = db.prepare(`SELECT * FROM projects WHERE dir = ?`).get(options.dir) as any;
  return {
    id: row.id,
    name: row.name,
    path: row.dir,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export function getProject(db: DatabaseType, projectId: string): ProjectInfo | null {
  const row = db.prepare(`SELECT * FROM projects WHERE id = ?`).get(projectId) as any;
  if (!row) return null;
  return {
    id: row.id,
    name: row.name,
    path: row.dir,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export function getProjectByDir(db: DatabaseType, dir: string): ProjectInfo | null {
  const row = db.prepare(`SELECT * FROM projects WHERE dir = ?`).get(dir) as any;
  if (!row) return null;
  return {
    id: row.id,
    name: row.name,
    path: row.dir,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export function listProjects(db: DatabaseType): ProjectInfo[] {
  const rows = db.prepare(`SELECT * FROM projects ORDER BY updated_at DESC`).all() as any[];
  return rows.map((row) => ({
    id: row.id,
    name: row.name,
    path: row.dir,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  }));
}

/**
 * Delete a project: close every session DB connection it owns, remove the
 * project's entire storage directory, then clear the global index rows.
 */
export function deleteProject(state: StorageState, projectId: string): boolean {
  const { globalDb, sessionDbs, storageDir } = state;

  // 1. Close cached session connections for this project.
  const sessionIds = globalDb
    .prepare(`SELECT id FROM sessions WHERE project_id = ?`)
    .all(projectId) as Array<{ id: string }>;
  for (const { id } of sessionIds) {
    const db = sessionDbs.get(id);
    if (db) {
      try {
        db.close();
      } catch {
        // Ignored
      }
      sessionDbs.delete(id);
    }
  }

  // 2. Delete the project's storage directory (sessions + DBs).
  try {
    const projDir = path.join(storageDir, "projects", projectId);
    if (fs.existsSync(projDir)) {
      fs.rmSync(projDir, { recursive: true, force: true });
    }
  } catch {
    // Ignored
  }

  // 3. Clear global index rows.
  globalDb.prepare(`DELETE FROM sessions WHERE project_id = ?`).run(projectId);
  const info = globalDb.prepare(`DELETE FROM projects WHERE id = ?`).run(projectId);
  return info.changes > 0;
}
