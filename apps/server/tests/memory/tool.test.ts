/**
 * Functional tests for the `memory` tool.
 * Zero network/API calls — 0 credits used.
 */
import assert from "node:assert/strict";
import { MemoryRegistry } from "@/agent/src/memory/index.js";
import { createMemoryTool } from "@/agent/src/tools/memory.js";

console.log("Running memory tool tests...");

function text(res: unknown): string {
  return (res as { content: Array<{ text: string }> }).content[0]?.text ?? "";
}

// Use a fresh in-memory-backed registry per project id so tests don't share
// state. `:memory:` storageDir makes both project and global stores in-process.
const registry = new MemoryRegistry(":memory:");
const tool = createMemoryTool("proj-1", registry);

// 1. store requires content
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "store" }));
  assert.ok((res as { isError?: boolean }).isError);
  console.log("  ✅ store without content errors");
}

// 2. store succeeds, defaults scope to 'project'
let storedId = "";
{
  const res = await tool.execute(
    tool.inputSchema.parse({ op: "store", content: "User prefers dark mode", tags: ["preference"] }),
  );
  assert.ok(text(res).includes("Stored memory"));
  assert.ok(text(res).includes("scope: project"));
  const match = text(res).match(/Stored memory ([^ ]+)/);
  storedId = match?.[1] ?? "";
  assert.ok(storedId);
  console.log("  ✅ store succeeds and defaults to project scope");
}

// 3. recall requires query or tags
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "recall" }));
  assert.ok((res as { isError?: boolean }).isError);
  console.log("  ✅ recall without query/tags errors");
}

// 4. recall finds the stored entry by tag
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "recall", tags: ["preference"] }));
  assert.ok(text(res).includes("dark mode"));
  console.log("  ✅ recall by tag finds entry");
}

// 5. list returns all entries in scope
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "list" }));
  assert.ok(text(res).includes("dark mode"));
  console.log("  ✅ list returns entries");
}

// 6. edit requires id
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "edit", content: "x" }));
  assert.ok((res as { isError?: boolean }).isError);
  console.log("  ✅ edit without id errors");
}

// 7. edit updates content
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "edit", id: storedId, content: "User prefers light mode" }));
  assert.ok(text(res).includes("Updated memory"));
  const listed = await tool.execute(tool.inputSchema.parse({ op: "list" }));
  assert.ok(text(listed).includes("light mode"));
  console.log("  ✅ edit updates content");
}

// 8. delete requires id
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "delete" }));
  assert.ok((res as { isError?: boolean }).isError);
  console.log("  ✅ delete without id errors");
}

// 9. delete removes entry
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "delete", id: storedId }));
  assert.ok(text(res).includes("Deleted memory"));
  const listed = await tool.execute(tool.inputSchema.parse({ op: "list" }));
  assert.ok(text(listed).includes("No memories found"));
  console.log("  ✅ delete removes entry");
}

// 10. delete/edit on missing id errors
{
  const res = await tool.execute(tool.inputSchema.parse({ op: "delete", id: "nope" }));
  assert.ok((res as { isError?: boolean }).isError);
  console.log("  ✅ delete on missing id errors");
}

// 11. global scope is isolated from project scope
{
  await tool.execute(tool.inputSchema.parse({ op: "store", content: "Cross-project fact", scope: "global" }));
  const projectList = await tool.execute(tool.inputSchema.parse({ op: "list", scope: "project" }));
  const globalList = await tool.execute(tool.inputSchema.parse({ op: "list", scope: "global" }));
  assert.ok(!text(projectList).includes("Cross-project fact"));
  assert.ok(text(globalList).includes("Cross-project fact"));
  console.log("  ✅ global scope isolated from project scope");
}

// 12. project scope without a projectId errors clearly
{
  const projectlessTool = createMemoryTool(null, registry);
  const res = await projectlessTool.execute(projectlessTool.inputSchema.parse({ op: "list" }));
  assert.ok((res as { isError?: boolean }).isError);
  console.log("  ✅ project scope without projectId errors");
}

console.log("memory tool tests passed!\n");
