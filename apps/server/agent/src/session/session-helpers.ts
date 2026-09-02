import { Database as DatabaseConstructor, type Database as DatabaseType } from "bun:sqlite";
import fs from "node:fs";
import path from "node:path";
import { initSessionDatabase } from "./schema.js";
import type { StorageState } from "./utils.js";

export function getProjectStorageDir(storageDir: string, projectId: string): string {
  return path.join(storageDir, "projects", projectId);
}

export function getProjectSessionsDir(storageDir: string, projectId: string): string {
  return path.join(getProjectStorageDir(storageDir, projectId), "sessions");
}

export function getSessionDbPath(storageDir: string, projectId: string, sessionId: string): string {
  return path.join(getProjectSessionsDir(storageDir, projectId), `${sessionId}.db`);
}

export function getScratchSessionsDir(storageDir: string): string {
  return path.join(storageDir, "scratch", "sessions");
}

export function getScratchSessionDbPath(storageDir: string, sessionId: string): string {
  return path.join(getScratchSessionsDir(storageDir), `${sessionId}.db`);
}

export function isScratchProjectId(projectId: string | null | undefined): boolean {
  return projectId == null;
}

export function ensureDir(filePath: string): void {
  try {
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
  } catch {
    // Ignored if folder exists
  }
}

export function removeDbFile(dbPath: string): void {
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
export function findSessionDbPath(storageDir: string, sessionId: string): string | undefined {
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
  // Also check scratch sessions
  const scratchCandidate = getScratchSessionDbPath(storageDir, sessionId);
  if (fs.existsSync(scratchCandidate)) return scratchCandidate;
  return undefined;
}

export function getProjectIdBySessionId(globalDb: DatabaseType, sessionId: string): string | null {
  const row = globalDb.prepare("SELECT project_id FROM sessions WHERE id = ?").get(sessionId) as
    | { project_id: string }
    | undefined;
  return row ? row.project_id : null;
}

export function getSessionDb(
  state: StorageState,
  sessionId: string,
  projectId: string | null,
): DatabaseType {
  let db = state.sessionDbs.get(sessionId);
  if (!db) {
    const dbPath = projectId == null
      ? getScratchSessionDbPath(state.storageDir, sessionId)
      : getSessionDbPath(state.storageDir, projectId, sessionId);
    ensureDir(dbPath);
    db = new DatabaseConstructor(dbPath);
    initSessionDatabase(db);
    state.sessionDbs.set(sessionId, db);
  }
  return db;
}

export function bumpSessionUpdated(
  globalDb: DatabaseType,
  sessionId: string,
  now: number,
  added: number,
): void {
  globalDb
    .prepare(`UPDATE sessions SET updated_at = ?, message_count = message_count + ? WHERE id = ?`)
    .run(now, added, sessionId);
}
