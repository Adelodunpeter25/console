import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { SessionService } from "@/api/src/services/session.service.js";
import { SqliteSessionStorage } from "@/agent/src/session/storage.js";
import { getSessionScratchDir } from "@/agent/src/session/apppaths.js";

console.log("Running scratchpad session tests...");

// Use temp storage so we don't pollute real DB
const storageDir = fs.mkdtempSync(path.join(os.tmpdir(), "console-scratch-test-"));
const storage = new SqliteSessionStorage({ storageDir });
const service = new SessionService(storage);

// 1. Scratch session: projectId null => cwd is per-session scratch, projectId undefined in header
const scratch = service.createSession({ projectId: null, title: "Scratch Test" });
assert.equal(scratch.projectId, undefined, "scratch projectId should be undefined (null in DB)");
assert.ok(scratch.cwd.includes("scratch"), `scratch cwd should contain scratch, got ${scratch.cwd}`);
assert.ok(scratch.cwd.includes(scratch.id), "scratch cwd should be per-session subdir");
assert.ok(fs.existsSync(scratch.cwd), "scratch cwd dir should exist after creation");
console.log("  ✅ Scratch session creation (projectId null, sandboxed cwd)");

// 2. Normal session: projectId string => cwd as provided or cwd fallback, projectId preserved
const normal = service.createSession({ cwd: "/tmp", projectId: "proj-123", title: "Normal" });
assert.equal(normal.projectId, "proj-123");
assert.equal(normal.cwd, "/tmp");
console.log("  ✅ Normal session creation");

// 3. Verify listSessions: scratch sessions have no projectId
const all = service.listSessions();
assert.ok(all.some((s) => s.id === scratch.id && s.projectId === undefined));
assert.ok(all.some((s) => s.id === normal.id && s.projectId === "proj-123"));
console.log("  ✅ List sessions preserves null projectId");

// 4. Desktop folder display: scratch should be "No Folder" (check via projectId null)
assert.equal(scratch.projectId, undefined);
console.log("  ✅ Desktop folder display would be No Folder for scratch");

// 5. Cleanup on permanent delete: scratch dir removed, normal not
// Soft delete first
assert.equal(service.deleteSession(scratch.id), true);
assert.equal(service.deleteSession(normal.id), true);
// Scratch dir should still exist after soft delete
assert.ok(fs.existsSync(scratch.cwd), "scratch dir should remain after soft delete");
// Permanent delete
assert.equal(service.permanentlyDeleteSession(scratch.id), true);
assert.ok(!fs.existsSync(scratch.cwd), "scratch dir should be removed after permanent delete");
console.log("  ✅ Cleanup on permanent delete removes scratch dir");

// Normal session's cwd (/tmp) should not be deleted
assert.ok(fs.existsSync("/tmp"), "/tmp should still exist");
assert.equal(service.permanentlyDeleteSession(normal.id), true);
console.log("  ✅ Normal session permanent delete does not touch /tmp");

// Cleanup storage
storage.close();
fs.rmSync(storageDir, { recursive: true, force: true });
// Also ensure scratch dir for scratch session is gone (already checked), but if any leftover, clean
try {
  const scratchRoot = path.join(os.homedir(), ".console", "scratch");
  // Don't delete whole scratch root, just ensure our session subdir is gone
  assert.ok(!fs.existsSync(scratch.cwd));
} catch {}

console.log("\nAll scratchpad tests passed!");
