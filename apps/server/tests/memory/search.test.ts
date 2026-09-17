/**
 * Unit tests for recallMemories scoring.
 * Zero network/API calls — 0 credits used.
 */
import assert from "node:assert/strict";
import { recallMemories, type MemoryEntry } from "@/agent/src/memory/index.js";

console.log("Running recallMemories tests...");

function makeEntry(overrides: Partial<MemoryEntry>): MemoryEntry {
  return {
    id: overrides.id ?? "id",
    scope: "project",
    content: "",
    tags: [],
    createdAt: 0,
    updatedAt: 0,
    ...overrides,
  };
}

const entries: MemoryEntry[] = [
  makeEntry({ id: "1", content: "User prefers terse commit messages", tags: ["preference", "git"] }),
  makeEntry({ id: "2", content: "Uses bun instead of node", tags: ["tooling"] }),
  makeEntry({ id: "3", content: "Prefers single bundled PRs over many small ones", tags: ["preference"] }),
  makeEntry({ id: "4", content: "Unrelated fact about the weather", tags: [] }),
];

// 1. Tag match scores higher than text-only match
{
  const matches = recallMemories(entries, { text: "prefers", tags: ["preference"] });
  assert.equal(matches[0]?.entry.id, "1"); // tag + text match: 3 + 1 = 4
  assert.equal(matches.length, 2); // "1" and "3" both tag-match; "2"/"4" score 0
  console.log("  ✅ tag match outranks text-only match");
}

// 2. Text-only match
{
  const matches = recallMemories(entries, { text: "instead of node" });
  assert.equal(matches.length, 1);
  assert.equal(matches[0]?.entry.id, "2");
  console.log("  ✅ text-only substring match");
}

// 3. Zero-score entries excluded
{
  const matches = recallMemories(entries, { text: "nonexistent-term" });
  assert.equal(matches.length, 0);
  console.log("  ✅ zero-score entries excluded");
}

// 4. Limit truncation
{
  const many: MemoryEntry[] = Array.from({ length: 20 }, (_, i) =>
    makeEntry({ id: `t${i}`, content: "match me", tags: [] }),
  );
  const matches = recallMemories(many, { text: "match" }, 5);
  assert.equal(matches.length, 5);
  console.log("  ✅ limit truncation");
}

// 5. Empty query yields no matches
{
  const matches = recallMemories(entries, {});
  assert.equal(matches.length, 0);
  console.log("  ✅ empty query yields no matches");
}

console.log("recallMemories tests passed!\n");
