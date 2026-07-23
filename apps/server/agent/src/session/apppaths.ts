import path from "node:path";
import os from "node:os";

/**
 * Returns the root directory path for Console storage based on the environment.
 * Uses ~/.console-dev in development and ~/.console in production.
 */
export function getConsoleStorageDir(): string {
  const isDev = process.env.NODE_ENV === "development" || process.env.CONSOLE_ENV === "dev";
  const homeDir = os.homedir();
  const folderName = isDev ? ".console-dev" : ".console";
  return path.join(homeDir, folderName);
}

/**
 * Returns the path to the global SQLite database.
 */
export function getGlobalDbPath(): string {
  return path.join(getConsoleStorageDir(), "console-global.db");
}

/**
 * Returns the directory path for a specific project's storage.
 */
export function getProjectStorageDir(projectId: string): string {
  return path.join(getConsoleStorageDir(), "projects", projectId);
}

/**
 * Returns the path to a specific project's session SQLite database.
 */
export function getProjectDbPath(projectId: string): string {
  return path.join(getProjectStorageDir(projectId), "project-sessions.db");
}
