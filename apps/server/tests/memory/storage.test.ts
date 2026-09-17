/**
 * Unit tests for SqliteMemoryStorage.
 * Zero network/API calls — 0 credits used.
 */
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { SqliteMemoryStorage } from "@/agent/src/memory/index.js";

console.log("Running SqliteMemoryStorage tests...");

const storage = new SqliteMemoryStorage(":memory:", "project");

// 1. Store
const entry = storage.store({ content: "User prefers dark mode", tags: ["preference", "ui"] });
assert.ok(entry.id);
assert.equal(entry.content, "User prefers dark mode");
assert.deepEqual(entry.tags, ["preference", "ui"]);
assert.equal(entry.scope, "project");
console.log("  ✅ store");

// 2. Get
const fetched = storage.get(entry.id);
assert.ok(fetched);
assert.equal(fetched?.content, entry.content);
console.log("  ✅ get");

// 3. List (with and without tag filter)
storage.store({ content: "Uses bun, not node", tags: ["tooling"] });
assert.equal(storage.list().length, 2);
assert.equal(storage.list({ tags: ["preference"] }).length, 1);
console.log("  ✅ list + tag filter");

// 4. Update
const updated = storage.update(entry.id, { content: "User prefers light mode" });
assert.equal(updated?.content, "User prefers light mode");
assert.ok((updated?.updatedAt ?? 0) >= entry.updatedAt);
assert.equal(storage.get(entry.id)?.content, "User prefers light mode");
console.log("  ✅ update");

// 5. Update missing id
assert.equal(storage.update("does-not-exist", { content: "x" }), null);
console.log("  ✅ update missing id returns null");

// 6. Delete
assert.equal(storage.remove(entry.id), true);
assert.equal(storage.get(entry.id), null);
assert.equal(storage.remove(entry.id), false);
console.log("  ✅ delete + idempotent delete");

storage.close();

// 7. Persistence across re-instantiation (temp file, not :memory:)
const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "console-memory-test-"));
const dbPath = path.join(tmpDir, "nested", "memory.db");
const first = new SqliteMemoryStorage(dbPath, "project");
first.store({ content: "Persisted fact", tags: [] });
first.close();

const second = new SqliteMemoryStorage(dbPath, "project");
assert.equal(second.list().length, 1);
assert.equal(second.list()[0]?.content, "Persisted fact");
second.close();
fs.rmSync(tmpDir, { recursive: true, force: true });
console.log("  ✅ persists across re-instantiation and creates nested dir");

console.log("SqliteMemoryStorage tests passed!\n");
