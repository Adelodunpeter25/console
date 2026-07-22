/**
 * Unit Tests for SqliteSessionStorage (Phase 4).
 * Uses in-memory SQLite database (`:memory:`).
 */
import assert from "node:assert/strict";
import { SqliteSessionStorage } from "../server/agent/src/session/index.js";
import type { AgentMessage } from "../server/agent/src/types/index.js";

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

// 3. Load session
const loaded = storage.loadSession(header.id);
assert.ok(loaded);
assert.equal(loaded.header.id, header.id);
assert.equal(loaded.messages.length, 2);
assert.equal(loaded.messages[0]?.role, "user");
assert.equal(loaded.messages[1]?.role, "assistant");
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

// 6. Delete session
assert.equal(storage.deleteSession(header.id), true);
assert.equal(storage.loadSession(header.id), null);
assert.equal(storage.listSessions().length, 0);
console.log("  ✅ Delete session");

storage.close();
console.log("SqliteSessionStorage tests passed!\n");
