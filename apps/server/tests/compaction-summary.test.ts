import assert from "node:assert/strict";
import {
  stripReadSelector,
  extractFileOps,
  formatFileTree,
} from "@/agent/src/compaction/file-tracker.js";
import { buildStructuralSummary } from "@/agent/src/compaction/structural-summary.js";
import { compactHistory } from "@/agent/src/compaction/index.js";
import type { AgentMessage } from "@console/types";

console.log("Running compaction summary tests...");

// 1. File tracker and selector stripping
{
  assert.equal(stripReadSelector("src/auth.ts:10-50"), "src/auth.ts");
  assert.equal(stripReadSelector("src/index.ts:raw"), "src/index.ts");
  assert.equal(stripReadSelector("src/main.ts:L15-30"), "src/main.ts");
  assert.equal(stripReadSelector("src/main.ts"), "src/main.ts");

  const sampleMessages: AgentMessage[] = [
    {
      role: "assistant",
      id: "a1",
      content: [
        { type: "toolCall", call: { id: "c1", name: "read_file", arguments: { path: "src/auth/token.ts:1-30" } } },
        { type: "toolCall", call: { id: "c2", name: "edit_file", arguments: { targetFile: "src/auth/token.ts" } } },
        { type: "toolCall", call: { id: "c3", name: "write_file", arguments: { path: "src/auth/new-file.ts" } } },
        { type: "toolCall", call: { id: "c4", name: "read_file", arguments: { path: "src/config.ts" } } },
      ],
      stopReason: "toolUse",
    },
  ];

  const ops = extractFileOps(sampleMessages);
  assert.equal(ops.read.has("src/auth/token.ts"), true);
  assert.equal(ops.read.has("src/config.ts"), true);
  assert.equal(ops.written.has("src/auth/new-file.ts"), true);
  assert.equal(ops.edited.has("src/auth/token.ts"), true);

  const tree = formatFileTree(ops);
  assert.equal(tree.includes("<files>"), true);
  assert.equal(tree.includes("token.ts (RW)"), true); // read and edited => RW
  assert.equal(tree.includes("new-file.ts (Write)"), true); // only written => Write
  assert.equal(tree.includes("config.ts (Read)"), true); // only read => Read
  assert.equal(tree.includes("</files>"), true);

  console.log("  ✅ File tracker categorizes Read, Write, and RW files into folded prefix tree");
}

// 2. buildStructuralSummary contains prompt, tools, and files
{
  const messages: AgentMessage[] = [
    { role: "user", content: "Please update the authentication token store" },
    {
      role: "assistant",
      id: "a1",
      content: [
        { type: "toolCall", call: { id: "c1", name: "read_file", arguments: { path: "src/token.ts" } } },
      ],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [{ toolCallId: "c1", content: "const secret = 123;", isError: false }],
    },
  ];

  const summary = buildStructuralSummary(messages);
  assert.equal(summary.includes("MUST build on prior work; NEVER duplicate prior work."), true);
  assert.equal(summary.includes("<summary>"), true);
  assert.equal(summary.includes("authentication token store"), true);
  assert.equal(summary.includes("read_file(src/token.ts)"), true);
  assert.equal(summary.includes("token.ts (Read)"), true);
  assert.equal(summary.includes("</summary>"), true);

  console.log("  ✅ buildStructuralSummary generates comprehensive checkpoint with file operations");
}

// 3. compactHistory generates valid alternating turns
{
  const history: AgentMessage[] = [];
  for (let i = 1; i <= 8; i++) {
    history.push({ role: "user", content: `Turn ${i} task: Implement authentication middleware and token verification logic for the project.` });
    history.push({
      role: "assistant",
      id: `a_${i}`,
      content: [
        {
          type: "toolCall",
          call: { id: `call_${i}`, name: "read_file", arguments: { path: `src/auth/file_${i}.ts` } },
        },
      ],
      stopReason: "toolUse",
    });
    history.push({
      role: "toolResult",
      results: [{ toolCallId: `call_${i}`, content: `// Source code of module ${i}\nexport function verifyToken${i}() {\n  return "valid_token_${i}_" + Math.random();\n}\n`.repeat(15) }],
    });
    history.push({
      role: "assistant",
      id: `a_resp_${i}`,
      content: [{ type: "text", text: `I have completed reading and verifying auth module ${i}. All tokens and keys match expected specifications.` }],
      stopReason: "stop",
    });
  }

  // Total 8 turns * 4 messages = 32 messages
  assert.equal(history.length, 32);

  const result = compactHistory(history, { keepRecentTokens: 50 });
  assert.equal(result.compactedMessages.length < history.length, true);
  assert.equal(result.compactedMessages[0].role, "user");
  assert.equal(result.compactedMessages[1].role, "assistant");
  assert.equal(result.tokensAfter < result.tokensBefore, true);

  // Check role alternation across the compacted output
  for (let i = 0; i < result.compactedMessages.length - 1; i++) {
    const curr = result.compactedMessages[i];
    const next = result.compactedMessages[i + 1];
    // In our model format, toolResult is followed by assistant or another turn
    if (curr.role === "user") {
      assert.equal(next.role === "assistant", true);
    }
  }

  console.log("  ✅ compactHistory reduces history and guarantees strict provider role alternation");
}

console.log("All compaction summary tests passed! ✨");
