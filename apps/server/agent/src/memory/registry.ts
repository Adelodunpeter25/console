/**
 * Resolves (scope, projectId) to the right SqliteMemoryStorage instance.
 * `tools/memory.ts` only ever talks to this module — never opens a DB file
 * directly.
 *
 * Layout:
 *   <storage>/projects/<projectId>/memory.db   (scope: "project")
 *   <storage>/memory-global.db                 (scope: "global", one shared file)
 */
import path from "node:path";
import { getConsoleStorageDir } from "../session/apppaths.js";
import { SqliteMemoryStorage } from "./storage.js";

const MAX_CACHED_PROJECT_STORES = 50;

export function getProjectMemoryDbPath(storageDir: string, projectId: string): string {
  return path.join(storageDir, "projects", projectId, "memory.db");
}

export function getGlobalMemoryDbPath(storageDir: string): string {
  return path.join(storageDir, "memory-global.db");
}

export class MemoryRegistry {
  private readonly storageDir: string;
  private readonly projectStores = new Map<string, SqliteMemoryStorage>();
  private globalStore: SqliteMemoryStorage | null = null;

  constructor(storageDir: string = getConsoleStorageDir()) {
    this.storageDir = storageDir;
  }

  forProject(projectId: string): SqliteMemoryStorage {
    const cached = this.projectStores.get(projectId);
    if (cached) {
      this.projectStores.delete(projectId);
      this.projectStores.set(projectId, cached);
      return cached;
    }
    const dbPath =
      this.storageDir === ":memory:" ? ":memory:" : getProjectMemoryDbPath(this.storageDir, projectId);
    const store = new SqliteMemoryStorage(dbPath, "project");
    this.projectStores.set(projectId, store);
    if (this.projectStores.size > MAX_CACHED_PROJECT_STORES) {
      const oldest = this.projectStores.keys().next().value as string | undefined;
      if (oldest !== undefined && oldest !== projectId) {
        this.projectStores.get(oldest)?.close();
        this.projectStores.delete(oldest);
      }
    }
    return store;
  }

  forGlobal(): SqliteMemoryStorage {
    if (!this.globalStore) {
      const dbPath = this.storageDir === ":memory:" ? ":memory:" : getGlobalMemoryDbPath(this.storageDir);
      this.globalStore = new SqliteMemoryStorage(dbPath, "global");
    }
    return this.globalStore;
  }

  resolve(scope: "project" | "global", projectId: string | null): SqliteMemoryStorage {
    if (scope === "global") return this.forGlobal();
    if (!projectId) {
      throw new Error("Memory scope 'project' requires a projectId; this session has no project.");
    }
    return this.forProject(projectId);
  }
}
