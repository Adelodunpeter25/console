import { Database as DatabaseConstructor, type Database as DatabaseType } from "bun:sqlite";
import fs from "node:fs";
import path from "node:path";
import { initSessionDatabase } from "./schema.js";
import { MAX_CACHED_SESSION_DBS, type StorageState } from "./utils.js";

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

export function getProjectIdBySessionId(globalDb: DatabaseType, sessionId: string): string | null | undefined {
  const row = globalDb.prepare("SELECT project_id FROM sessions WHERE id = ?").get(sessionId) as
    | { project_id: string | null }
    | undefined;
  if (!row) return undefined;
  return row.project_id && row.project_id !== "scratch" ? row.project_id : null;
}

export function getSessionDb(
  state: StorageState,
  sessionId: string,
  projectId: string | null,
): DatabaseType {
  const cached = state.sessionDbs.get(sessionId);
  if (cached) {
    // Refresh LRU order on hit: delete + re-set moves the entry to the end.
    state.sessionDbs.delete(sessionId);
    state.sessionDbs.set(sessionId, cached);
    return cached;
  }
  let db: DatabaseType;
  if (state.storageDir === ":memory:") {
    db = new DatabaseConstructor(":memory:");
  } else {
    const isScratch = projectId == null || projectId === "" || projectId === "scratch";
    const dbPath = isScratch
      ? getScratchSessionDbPath(state.storageDir, sessionId)
      : getSessionDbPath(state.storageDir, projectId, sessionId);
    ensureDir(dbPath);
    db = new DatabaseConstructor(dbPath);
  }
  initSessionDatabase(db);
  state.sessionDbs.set(sessionId, db);
  // Evict the least-recently-used session DB so a long-lived server never
  // accumulates an unbounded set of open SQLite handles.
  if (state.sessionDbs.size > MAX_CACHED_SESSION_DBS) {
    const oldest = state.sessionDbs.keys().next().value as string | undefined;
    if (oldest !== undefined && oldest !== sessionId) {
      evictSessionDb(state, oldest);
    }
  }
  return db;
}

/**
 * Move a session's SQLite file (plus WAL sidecars) from one path to another.
 * Used when a session's project changes (the file location records project
 * ownership) and to self-heal orphaned files found by `findSessionDbPath`.
 * Never clobbers an existing file at the destination. Returns true when a
 * move happened.
 */
export function relocateSessionDb(
  state: StorageState,
  sessionId: string,
  fromPath: string,
  toPath: string,
): boolean {
  if (fromPath === toPath) return false;
  if (!fs.existsSync(fromPath)) return false;
  // Never overwrite: a file already at the destination may hold newer turns
  // (e.g. written before an older build moved the index entry).
  if (fs.existsSync(toPath)) return false;
  // Checkpoint the cached handle so WAL frames are folded into the main
  // DB before the copy; without this the copy is a stale empty image (the
  // message rows only exist in the -wal). Keep the handle open during the
  // copy (safer than evict-then-copy: a fresh open can hit SQLITE_IOERR on
  // macOS when the file was just renamed), then evict it so later accesses
  // open the new path.
  const cached = state.sessionDbs.get(sessionId);
  if (cached) {
    try {
      cached.exec("PRAGMA wal_checkpoint(TRUNCATE)");
    } catch {
      try {
        cached.exec("PRAGMA wal_checkpoint(PASSIVE)");
      } catch {
        // Best-effort — the copy below still proceeds.
      }
    }
  }
  try {
    ensureDir(toPath);
    fs.copyFileSync(fromPath, toPath);
    for (const ext of ["-wal", "-shm"]) {
      try {
        fs.rmSync(`${toPath}${ext}`, { force: true });
      } catch {
        // Absent sidecars are the normal case; ignore.
      }
    }
    // Evict AFTER the copy so the handle can't write to the source inode
    // mid-copy; later accesses reopen at the new path via `getSessionDb`.
    evictSessionDb(state, sessionId);
    try {
      fs.rmSync(fromPath, { force: true });
      for (const ext of ["-wal", "-shm"]) {
        try {
          fs.rmSync(`${fromPath}${ext}`, { force: true });
        } catch {
          // Sidecars are recoverable; the main DB copy already succeeded.
        }
      }
    } catch {
      // The destination copy is complete even if stale source cleanup fails.
    }
    return true;
  } catch {
    try {
      fs.rmSync(toPath, { force: true });
    } catch {
      // Partial copy cleanup is best-effort.
    }
    return false;
  }
}

/**
 * Close and drop one cached per-session DB handle.
 * Safe to call for sessions with no cached handle (no-op).
 */
export function evictSessionDb(state: StorageState, sessionId: string): void {
  const db = state.sessionDbs.get(sessionId);
  if (!db) return;
  state.sessionDbs.delete(sessionId);
  try {
    db.close();
  } catch {
    // Ignored — eviction is best-effort; the file stays on disk.
  }
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
