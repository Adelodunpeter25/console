/**
 * Shared helpers and types for session storage.
 */
import type { AgentMessage } from "@/agent/src/types/index.js";

/**
 * Upper bound on open per-session SQLite handles kept in `StorageState`.
 * Each handle holds an FD (plus -wal/-shm while checkpointing), so an
 * unbounded `sessionDbs` map leaks FDs across a long-lived server process.
 * The limit only affects the cache: evicted sessions reopen their DB file
 * on next access via `getSessionDb`.
 */
export const MAX_CACHED_SESSION_DBS = 50;

const MAX_PERSIST_CHARS = 500_000;
const TRUNCATION_NOTICE = "\n\n[Session persistence truncated large content]";

/**
 * Truncate strings in message content if they exceed safety limits (500k chars).
 */
export function truncateForPersistence(message: AgentMessage): AgentMessage {
  if (message.role === "toolResult") {
    const truncatedResults = message.results.map((res) => {
      if (typeof res.content === "string" && res.content.length > MAX_PERSIST_CHARS) {
        return {
          ...res,
          content: res.content.slice(0, MAX_PERSIST_CHARS) + TRUNCATION_NOTICE,
        };
      }
      return res;
    });
    return { ...message, results: truncatedResults };
  }
  return message;
}

/** Row shape for the single `session_meta` row in a per-session DB. */
export interface SessionMetaRow {
  title: string;
  cwd: string;
  project_id: string | null;
  model_id: string;
  provider: string;
  approval_mode: string | null;
  repaired: number;
  created_at: number;
  updated_at: number;
}

/** Row shape for a session in the global `sessions` index table. */
export interface SessionIndexRow {
  id: string;
  title: string;
  cwd: string;
  project_id: string | null;
  model_id: string;
  provider: string;
  approval_mode: string | null;
  message_count: number;
  status: string | null;
  created_at: number;
  updated_at: number;
  deleted_at: number | null;
}

/**
 * Mutable state shared across session/project operation modules.
 * The `SqliteSessionStorage` class constructs and owns one of these.
 */
export interface StorageState {
  globalDb: import("bun:sqlite").Database;
  sessionDbs: Map<string, import("bun:sqlite").Database>;
  storageDir: string;
}
