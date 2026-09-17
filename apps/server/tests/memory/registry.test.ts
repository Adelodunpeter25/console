/**
 * Unit tests for MemoryRegistry resolution.
 * Zero network/API calls — 0 credits used.
 */
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { MemoryRegistry, getGlobalMemoryDbPath, getProjectMemoryDbPath } from "@/agent/src/memory/index.js";

console.log("Running MemoryRegistry tests...");

const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "console-memory-registry-"));
const registry = new MemoryRegistry(tmpDir);

// 1. Distinct project stores per projectId
{
  const storeA = registry.forProject("proj-a");
  const storeB = registry.forProject("proj-b");
  storeA.store({ content: "A-only fact" });
  assert.equal(storeA.list().length, 1);
  assert.equal(storeB.list().length, 0);
  console.log("  ✅ distinct stores per projectId");
}

// 2. Same instance returned for repeated calls (cache hit)
{
  const first = registry.forProject("proj-a");
  const second = registry.forProject("proj-a");
  assert.equal(first, second);
  console.log("  ✅ cached instance reused");
}

// 3. Global store is a single shared singleton
{
  const g1 = registry.forGlobal();
  const g2 = registry.forGlobal();
  assert.equal(g1, g2);
  g1.store({ content: "global fact" });
  assert.equal(g2.list().length, 1);
  console.log("  ✅ global store is a shared singleton");
}

// 4. resolve() routes correctly, errors on project scope without projectId
{
  assert.equal(registry.resolve("global", null), registry.forGlobal());
  assert.equal(registry.resolve("project", "proj-a"), registry.forProject("proj-a"));
  assert.throws(() => registry.resolve("project", null));
  console.log("  ✅ resolve() routes scope+projectId, throws without projectId");
}

// 5. Path helpers point inside the project dir / storage root respectively
{
  assert.equal(getProjectMemoryDbPath(tmpDir, "proj-a"), path.join(tmpDir, "projects", "proj-a", "memory.db"));
  assert.equal(getGlobalMemoryDbPath(tmpDir), path.join(tmpDir, "memory-global.db"));
  assert.ok(fs.existsSync(getProjectMemoryDbPath(tmpDir, "proj-a")));
  assert.ok(fs.existsSync(getGlobalMemoryDbPath(tmpDir)));
  console.log("  ✅ path helpers match on-disk layout");
}

fs.rmSync(tmpDir, { recursive: true, force: true });
console.log("MemoryRegistry tests passed!\n");
