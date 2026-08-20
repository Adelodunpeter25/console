/**
 * Verifies the cwd-lock guard in SessionService.updateSession:
 * a session's project/cwd may be changed only while it has no messages.
 * Once messages exist, cwd updates are silently ignored (run integrity).
 */
import assert from "node:assert/strict";
import { SqliteSessionStorage } from "../agent/src/session/index.js";
import { SessionService } from "../api/src/services/session.service.js";
import type { AgentMessage } from "@console/types";

console.log("Running SessionService cwd-lock tests...");

const storage = new SqliteSessionStorage({ dbPath: ":memory:" });
const service = new SessionService(storage);

// 1. Create a session.
const header = service.createSession({
  cwd: "/projects/alpha",
  modelId: "gemini-2.5-pro",
  provider: "antigravity",
  title: "Cwd Lock Test",
});
assert.ok(header.id);
assert.equal(header.cwd, "/projects/alpha");
console.log("  ✅ Session created with /projects/alpha");

// 2. Before any messages: cwd change is allowed.
const beforeMsg = service.updateSession(header.id, { cwd: "/projects/beta" });
assert.equal(beforeMsg?.cwd, "/projects/beta");
console.log("  ✅ cwd change allowed before messages (/projects/beta)");

// 3. Add a user message — simulates a chat that has started.
const userMsg: AgentMessage = { role: "user", content: "hello" };
storage.appendMessage(header.id, userMsg);
assert.equal(storage.loadSession(header.id)?.messages.length, 1);
console.log("  ✅ One message appended");

// 4. After messages: cwd change is silently ignored. Other fields (title)
//    are still applied, and the returned header still resolves 200.
const afterMsg = service.updateSession(header.id, {
  cwd: "/projects/gamma",
  title: "Renamed Mid-Chat",
});
assert.equal(afterMsg?.cwd, "/projects/beta", "cwd must NOT change after messages");
assert.equal(afterMsg?.title, "Renamed Mid-Chat", "title update still applies");
console.log("  ✅ cwd change ignored after messages; title still updated");

// 5. Confirm the persisted header also retains the locked cwd.
const reloaded = service.getSession(header.id);
assert.equal(reloaded?.header.cwd, "/projects/beta");
console.log("  ✅ Persisted header retains locked cwd after reload");

console.log("\nAll cwd-lock tests passed.");
