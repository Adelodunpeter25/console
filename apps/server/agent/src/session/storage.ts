/**
 * SQLite-backed Session & History Storage Engine (WAL mode).
 *
 * Thin facade over the project (`projects.ts`) and session (`sessions.ts`)
 * operation modules. This class owns the database connections and storage
 * directory, then delegates each operation to the relevant module so the
 * file stays short and navigable.
 *
 * Layout:
 * - `<storage>/console-global.db` — `projects` + `sessions` index tables.
 * - `<storage>/projects/<projectId>/sessions/<sessionId>.db` — one file per
 *   session with `session_meta` + `messages`. The file location records
 *   project ownership; `model_id` and `provider` are persisted in meta.
 */
import DatabaseConstructor, { type Database as DatabaseType } from "better-sqlite3";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { AgentMessage, SessionHeader, ProjectInfo } from "../types/index.js";
import { initGlobalDatabase } from "./schema.js";
import { getGlobalDbPath, getConsoleStorageDir } from "./apppaths.js";
import { type StorageState } from "./utils.js";
import * as Projects from "./projects.js";
import * as Sessions from "./sessions.js";

export class SqliteSessionStorage {
  private state: StorageState;

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

    const globalDb = new DatabaseConstructor(globalDbPath);
    initGlobalDatabase(globalDb);

    let storageDir: string;
    if (options?.storageDir) {
      storageDir = options.storageDir;
    } else if (options?.dbPath === ":memory:") {
      storageDir = fs.mkdtempSync(path.join(os.tmpdir(), "console-storage-"));
    } else {
      storageDir = getConsoleStorageDir();
    }

    this.state = {
      globalDb,
      sessionDbs: new Map<string, DatabaseType>(),
      storageDir,
    };
  }

  // MARK: - Projects

  createProject(options: { id?: string; name: string; dir: string }): ProjectInfo {
    return Projects.createProject(this.state.globalDb, options);
  }

  getProject(projectId: string): ProjectInfo | null {
    return Projects.getProject(this.state.globalDb, projectId);
  }

  getProjectByDir(dir: string): ProjectInfo | null {
    return Projects.getProjectByDir(this.state.globalDb, dir);
  }

  listProjects(): ProjectInfo[] {
    return Projects.listProjects(this.state.globalDb);
  }

  deleteProject(projectId: string): boolean {
    return Projects.deleteProject(this.state, projectId);
  }

  // MARK: - Sessions

  createSession(options: {
    id?: string;
    title?: string;
    cwd: string;
    projectId?: string;
    modelId: string;
    provider: string;
  }): SessionHeader {
    return Sessions.createSession(this.state, options);
  }

  appendMessage(sessionId: string, message: AgentMessage): void {
    Sessions.appendMessage(this.state, sessionId, message);
  }

  appendMessages(sessionId: string, messages: AgentMessage[]): void {
    Sessions.appendMessages(this.state, sessionId, messages);
  }

  loadSession(sessionId: string): { header: SessionHeader; messages: AgentMessage[] } | null {
    return Sessions.loadSession(this.state, sessionId);
  }

  listSessions(options?: { cwd?: string; projectId?: string; limit?: number }): SessionHeader[] {
    return Sessions.listSessions(this.state.globalDb, options);
  }

  deleteSession(sessionId: string): boolean {
    return Sessions.deleteSession(this.state, sessionId);
  }

  updateSessionStatus(sessionId: string, status: string): void {
    Sessions.updateSessionStatus(this.state.globalDb, sessionId, status);
  }

  updateTitle(sessionId: string, title: string): boolean {
    return Sessions.updateTitle(this.state, sessionId, title);
  }

  updateModel(sessionId: string, modelId: string, provider: string): boolean {
    return Sessions.updateModel(this.state, sessionId, modelId, provider);
  }

  // MARK: - Lifecycle

  clearAll(): void {
    Sessions.clearAll(this.state);
  }

  close(): void {
    this.state.globalDb.close();
    for (const [, db] of this.state.sessionDbs) {
      try {
        db.close();
      } catch {
        // Ignored
      }
    }
    this.state.sessionDbs.clear();
  }
}
