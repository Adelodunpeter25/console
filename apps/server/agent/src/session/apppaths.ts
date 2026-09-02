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
 * @deprecated Per-session DBs (`getSessionDbPath`) replace project-level DBs.
 */
export function getProjectDbPath(projectId: string): string {
  return path.join(getProjectStorageDir(projectId), "project-sessions.db");
}

/**
 * Returns the directory holding one SQLite file per session for a project.
 * Sessions are linked to their project on disk: each session DB lives at
 * `<storage>/projects/<projectId>/sessions/<sessionId>.db`.
 */
export function getProjectSessionsDir(projectId: string): string {
  return path.join(getProjectStorageDir(projectId), "sessions");
}

/**
 * Returns the path to a single session's SQLite database, scoped to its
 * project. Each session is fully self-contained: header metadata + message
 * history, and the file location records project ownership.
 */
export function getSessionDbPath(projectId: string, sessionId: string): string {
  return path.join(getProjectSessionsDir(projectId), `${sessionId}.db`);
}

/**
 * Returns the scratchpad directory for projectless sessions.
 * Sandboxed cwd ensuring temporary files stay isolated.
 */
export function getScratchDir(): string {
  return path.join(getConsoleStorageDir(), "scratch");
}

/**
 * Returns the per-session scratch working directory.
 */
export function getSessionScratchDir(sessionId: string): string {
  return path.join(getScratchDir(), sessionId);
}
