/**
 * Unit Tests for SqliteSessionStorage (Phase 4).
 * Uses in-memory SQLite database (`:memory:`).
 */
import assert from "node:assert/strict";
import { SqliteSessionStorage } from "../agent/src/session/index.js";
import type { AgentMessage } from "@console/types";

console.log("Running SqliteSessionStorage tests...");

const storage = new SqliteSessionStorage({ dbPath: ":memory:" });

// 1. Create session
const header = storage.createSession({
  cwd: "/projects/test",
  modelId: "gemini-3.1-pro",
  provider: "antigravity",
  title: "Test Feature Work",
});

assert.ok(header.id);
assert.equal(header.title, "Test Feature Work");
assert.equal(header.cwd, "/projects/test");
assert.equal(header.modelId, "gemini-3.1-pro");
assert.equal(header.provider, "antigravity");
console.log("  ✅ Session creation");

// 2. Append messages
const userMsg: AgentMessage = { role: "user", content: "Implement login route" };
const assistantMsg: AgentMessage = {
  role: "assistant",
  id: "asst-1",
  content: [{ type: "text", text: "Working on it." }],
  stopReason: "stop",
};

storage.appendMessage(header.id, userMsg);
storage.appendMessage(header.id, assistantMsg);
console.log("  ✅ Append individual messages");

storage.replaceMessages(header.id, [userMsg, assistantMsg]);
assert.equal(storage.loadSession(header.id)?.messages.length, 2);
console.log("  ✅ Replace ordered session history");

// 2b. Incrementally upsert tool results without duplicating the message row.
storage.upsertToolResult(header.id, "tool-results:run-1:0", {
  toolCallId: "call-1",
  toolName: "readFile",
  content: "first result",
});
storage.upsertToolResult(header.id, "tool-results:run-1:0", {
  toolCallId: "call-2",
  toolName: "readFile",
  content: "second result",
});
storage.upsertToolResult(header.id, "tool-results:run-1:0", {
  toolCallId: "call-1",
  toolName: "readFile",
  content: "first result (replayed)",
});

const withToolResults = storage.loadSession(header.id);
assert.ok(withToolResults);
assert.equal(withToolResults.messages.length, 3);
const toolMessage = withToolResults.messages[2];
assert.equal(toolMessage?.role, "toolResult");
if (toolMessage?.role === "toolResult") {
  assert.equal(toolMessage.results.length, 2);
  assert.equal(toolMessage.results[0]?.content, "first result (replayed)");
}
console.log("  ✅ Incremental tool-result upsert");

// 2c. Cursor pagination returns the newest batch and older cursor.
const latestPage = storage.loadSessionPage(header.id, { limit: 2 });
assert.ok(latestPage);
assert.equal(latestPage.messages.length, 2);
assert.equal(latestPage.messages[0]?.role, "assistant");
assert.equal(latestPage.messages[1]?.role, "toolResult");
assert.equal(latestPage.hasMore, true);
assert.equal(typeof latestPage.nextCursor, "number");
if (latestPage.nextCursor !== null) {
  const olderPage = storage.loadSessionPage(header.id, {
    limit: 2,
    before: latestPage.nextCursor,
  });
  assert.ok(olderPage);
  assert.equal(olderPage.messages.length, 1);
  assert.equal(olderPage.messages[0]?.role, "user");
  assert.equal(olderPage.hasMore, false);
  assert.equal(olderPage.nextCursor, null);
}
console.log("  ✅ Cursor-paginated session history");

// 3. Load session
const loaded = storage.loadSession(header.id);
assert.ok(loaded);
assert.equal(loaded.header.id, header.id);
assert.equal(loaded.messages.length, 3);
assert.equal(loaded.messages[0]?.role, "user");
assert.equal(loaded.messages[1]?.role, "assistant");
assert.equal(loaded.messages[2]?.role, "toolResult");
console.log("  ✅ Load session history");

// 4. List sessions with cwd filter
const all = storage.listSessions();
assert.equal(all.length, 1);
assert.equal(all[0]?.id, header.id);

const filtered = storage.listSessions({ cwd: "/projects/test" });
assert.equal(filtered.length, 1);

const emptyList = storage.listSessions({ cwd: "/non-existent-path" });
assert.equal(emptyList.length, 0);
console.log("  ✅ List sessions & filter by cwd");

// 5. Update title & model
assert.equal(storage.updateTitle(header.id, "Renamed Session"), true);
assert.equal(storage.updateModel(header.id, "claude-sonnet-4-6", "antigravity"), true);

const reloaded = storage.loadSession(header.id);
assert.equal(reloaded?.header.title, "Renamed Session");
assert.equal(reloaded?.header.modelId, "claude-sonnet-4-6");
console.log("  ✅ Update session title & model");

// 6. Delete session (Soft Delete)
assert.equal(storage.deleteSession(header.id), true);
assert.equal(storage.loadSession(header.id), null);
assert.equal(storage.listSessions().length, 0);

// Test listing deleted sessions
const deletedSessions = storage.listSessions({ onlyDeleted: true });
assert.equal(deletedSessions.length, 1);
assert.equal(deletedSessions[0]?.id, header.id);
console.log("  ✅ Soft delete session & list onlyDeleted");

// 7. Restore session
assert.equal(storage.restoreSession(header.id), true);
assert.equal(storage.listSessions().length, 1);
const restored = storage.loadSession(header.id);
assert.ok(restored);
assert.equal(restored.header.title, "Renamed Session");
console.log("  ✅ Restore session");

// 8. Repair interrupted tool history once and skip subsequent checks.
const repairHeader = storage.createSession({
  cwd: "/projects/test",
  modelId: "gemini-3.1-pro",
  provider: "antigravity",
  title: "Repair Test",
});
storage.appendMessage(repairHeader.id, {
  role: "assistant",
  content: [
    {
      type: "toolCall",
      call: { id: "repair-call", name: "readFile", arguments: { path: "missing" } },
    },
  ],
});
assert.equal(storage.repairSession(repairHeader.id), true);
const repaired = storage.loadSession(repairHeader.id);
assert.ok(repaired);
assert.equal(repaired.messages.length, 2);
assert.equal(repaired.messages[1]?.role, "toolResult");
assert.equal(storage.repairSession(repairHeader.id), false);
assert.equal(storage.deleteSession(repairHeader.id), true);
console.log("  ✅ One-time interrupted-history repair");

storage.close();
console.log("SqliteSessionStorage tests passed!\n");
