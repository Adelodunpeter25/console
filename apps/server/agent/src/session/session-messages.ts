import crypto from "node:crypto";
import type { AgentMessage, ToolResult } from "../types/index.js";
import {
  bumpSessionUpdated,
  getProjectIdBySessionId,
  getSessionDb,
} from "./session-helpers.js";
import { truncateForPersistence, type StorageState } from "./utils.js";

export function appendMessage(state: StorageState, sessionId: string, message: AgentMessage): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const now = Date.now();
  const safeMsg = truncateForPersistence(message);
  const msgId = (safeMsg as any).id || crypto.randomUUID();
  const contentJson = JSON.stringify(safeMsg);

  const sessionDb = getSessionDb(state, sessionId, projectId);
  sessionDb
    .prepare(`INSERT INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`)
    .run(msgId, safeMsg.role, contentJson, now);

  bumpSessionUpdated(state.globalDb, sessionId, now, 1);
}

export function appendMessages(
  state: StorageState,
  sessionId: string,
  messages: AgentMessage[],
): void {
  if (messages.length === 0) return;
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const now = Date.now();
  const sessionDb = getSessionDb(state, sessionId, projectId);
  const insertMsg = sessionDb.prepare(
    `INSERT OR IGNORE INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`,
  );

  let inserted = 0;
  const tx = sessionDb.transaction(() => {
    for (const msg of messages) {
      const safeMsg = truncateForPersistence(msg);
      const msgId =
        (safeMsg as any).id ||
        crypto.createHash("sha256").update(JSON.stringify(safeMsg)).digest("hex").slice(0, 32);
      const info = insertMsg.run(msgId, safeMsg.role, JSON.stringify(safeMsg), now);
      if (info.changes > 0) inserted++;
    }
  });
  tx();

  bumpSessionUpdated(state.globalDb, sessionId, now, inserted);
}

/** Replace a session history after repairing an interrupted tool turn. */
export function replaceMessages(
  state: StorageState,
  sessionId: string,
  messages: AgentMessage[],
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const now = Date.now();
  const sessionDb = getSessionDb(state, sessionId, projectId);
  const insert = sessionDb.prepare(
    `INSERT INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`,
  );
  const transaction = sessionDb.transaction(() => {
    sessionDb.prepare("DELETE FROM messages").run();
    messages.forEach((message, index) => {
      const safeMessage = truncateForPersistence(message);
      const messageId =
        (safeMessage as any).id ||
        crypto.createHash("sha256").update(`${index}:${JSON.stringify(safeMessage)}`).digest("hex");
      insert.run(messageId, safeMessage.role, JSON.stringify(safeMessage), (safeMessage as any).createdAt ?? (now + index));
    });
  });
  transaction();

  state.globalDb
    .prepare("UPDATE sessions SET updated_at = ?, message_count = ? WHERE id = ?")
    .run(now, messages.length, sessionId);
}

/**
 * Incrementally persist a tool result into one stable tool-result message for
 * the active run. Replayed events are replaced by toolCallId, so persistence
 * remains idempotent across reconnects and retries.
 */
export function upsertToolResult(
  state: StorageState,
  sessionId: string,
  persistenceId: string,
  result: ToolResult,
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (!projectId) return;

  const now = Date.now();
  const sessionDb = getSessionDb(state, sessionId, projectId);
  const existing = sessionDb
    .prepare("SELECT content FROM messages WHERE id = ?")
    .get(persistenceId) as { content: string } | undefined;

  const message: Extract<AgentMessage, { role: "toolResult" }> = existing
    ? JSON.parse(existing.content)
    : { role: "toolResult", results: [] };
  const resultIndex = message.results.findIndex((item) => item.toolCallId === result.toolCallId);
  if (resultIndex >= 0) {
    message.results[resultIndex] = result;
  } else {
    message.results.push(result);
  }

  const safeMessage = truncateForPersistence(message);
  const content = JSON.stringify(safeMessage);
  if (existing?.content === content) return;

  sessionDb
    .prepare(
      `INSERT INTO messages (id, role, content, created_at)
       VALUES (?, ?, ?, ?)
       ON CONFLICT(id) DO UPDATE SET content = excluded.content`,
    )
    .run(persistenceId, safeMessage.role, content, now);

  bumpSessionUpdated(state.globalDb, sessionId, now, existing ? 0 : 1);
}
