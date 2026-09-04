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
import { Database as DatabaseConstructor, type Database as DatabaseType } from "bun:sqlite";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { AgentMessage, ModelFavorite, SessionHeader, ProjectInfo, ToolResult } from "@/agent/src/types/index.js";
import type { SessionFileChange, TodoItem } from "@console/types";
import { initGlobalDatabase } from "./schema.js";
import { getGlobalDbPath, getConsoleStorageDir } from "./apppaths.js";
import { MAX_CACHED_SESSION_DBS, type StorageState } from "./utils.js";
import { evictSessionDb } from "./session-helpers.js";
import * as Projects from "./projects.js";
import * as Sessions from "./sessions.js";
import type { CreateSessionOptions } from "./session-ops.js";
import * as ModelFavorites from "./model-favorites.js";

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

  createSession(options: CreateSessionOptions): SessionHeader {
    return Sessions.createSession(this.state, options);
  }

  appendMessage(sessionId: string, message: AgentMessage): void {
    Sessions.appendMessage(this.state, sessionId, message);
  }

  appendMessages(sessionId: string, messages: AgentMessage[]): void {
    Sessions.appendMessages(this.state, sessionId, messages);
  }

  replaceMessages(sessionId: string, messages: AgentMessage[]): void {
    Sessions.replaceMessages(this.state, sessionId, messages);
  }

  repairSession(sessionId: string): boolean {
    return Sessions.repairSession(this.state, sessionId);
  }

  markSessionNeedsRepair(sessionId: string): void {
    Sessions.markSessionNeedsRepair(this.state, sessionId);
  }

  upsertToolResult(sessionId: string, persistenceId: string, result: ToolResult): void {
    Sessions.upsertToolResult(this.state, sessionId, persistenceId, result);
  }

  loadSession(sessionId: string): { header: SessionHeader; messages: AgentMessage[] } | null {
    return Sessions.loadSession(this.state, sessionId);
  }

  loadSessionPage(
    sessionId: string,
    options: { limit?: number; before?: number },
  ): {
    header: SessionHeader;
    messages: AgentMessage[];
    hasMore: boolean;
    nextCursor: number | null;
  } | null {
    return Sessions.loadSession(this.state, sessionId, options);
  }

  listSessions(options?: { cwd?: string; projectId?: string; limit?: number; onlyDeleted?: boolean }): SessionHeader[] {
    return Sessions.listSessions(this.state.globalDb, options);
  }

  deleteSession(sessionId: string): boolean {
    return Sessions.deleteSession(this.state, sessionId);
  }

  restoreSession(sessionId: string): boolean {
    return Sessions.restoreSession(this.state, sessionId);
  }

  permanentlyDeleteSession(sessionId: string): boolean {
    return Sessions.permanentlyDeleteSession(this.state, sessionId);
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

  updateCwd(sessionId: string, cwd: string): boolean {
    return Sessions.updateCwd(this.state, sessionId, cwd);
  }

  updateApprovalMode(sessionId: string, approvalMode: string): boolean {
    return Sessions.updateApprovalMode(this.state, sessionId, approvalMode);
  }

  listModelFavorites() {
    return ModelFavorites.listModelFavorites(this.state.globalDb);
  }

  setModelFavorite(favorite: ModelFavorite, isFavorite: boolean): void {
    ModelFavorites.setModelFavorite(this.state.globalDb, favorite, isFavorite);
  }

  // MARK: - File Changes

  recordFileChange(sessionId: string, change: SessionFileChange): void {
    Sessions.recordFileChange(this.state, sessionId, change);
  }

  getSessionFileChanges(sessionId: string): SessionFileChange[] {
    return Sessions.getSessionFileChanges(this.state, sessionId);
  }

  clearSessionFileChanges(sessionId: string): void {
    Sessions.clearSessionFileChanges(this.state, sessionId);
  }

  // MARK: - Todos

  saveSessionTodos(sessionId: string, items: readonly TodoItem[]): void {
    Sessions.saveSessionTodos(this.state, sessionId, items);
  }

  getSessionTodos(sessionId: string): TodoItem[] {
    return Sessions.getSessionTodos(this.state, sessionId);
  }

  clearSessionTodos(sessionId: string): void {
    Sessions.clearSessionTodos(this.state, sessionId);
  }

  upsertSubagentStart(sessionId: string, event: import("@console/types").SubagentStartEvent): void {
    Sessions.upsertSubagentStart(this.state, sessionId, event);
  }

  appendSubagentActivity(sessionId: string, event: import("@console/types").SubagentActivityEvent): void {
    Sessions.appendSubagentActivity(this.state, sessionId, event);
  }

  completeSubagent(sessionId: string, event: import("@console/types").SubagentEndEvent): void {
    Sessions.completeSubagent(this.state, sessionId, event);
  }

  getSessionSubagents(sessionId: string): import("@console/types").SubagentInfo[] {
    return Sessions.getSessionSubagents(this.state, sessionId);
  }

  // MARK: - Lifecycle

  /**
   * Drop one cached per-session handle so idle sessions don't pin FDs.
   * The DB file stays on disk; the next access reopens it via `getSessionDb`.
   */
  releaseSession(sessionId: string): void {
    evictSessionDb(this.state, sessionId);
  }

  /** Number of open per-session handles (test hook for the LRU bound). */
  cachedSessionCount(): number {
    return this.state.sessionDbs.size;
  }

  /** Exposed for tests: the cap enforced by `getSessionDb`. */
  static maxCachedSessionDbs(): number {
    return MAX_CACHED_SESSION_DBS;
  }

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
