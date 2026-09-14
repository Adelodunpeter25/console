import assert from "node:assert/strict";
import { findCutPoint, isToolCallSafe } from "@/agent/src/compaction/cut-point.js";
import type { AgentMessage } from "@console/types";

console.log("Running compaction cut-point tests...");

// 1. isToolCallSafe validation
{
  const history: AgentMessage[] = [
    { role: "user", content: "Prompt 1" },
    {
      role: "assistant",
      id: "a1",
      content: [{ type: "toolCall", call: { id: "call_abc", name: "read_file", arguments: {} } }],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [{ toolCallId: "call_abc", content: "file content" }],
    },
    { role: "assistant", id: "a2", content: [{ type: "text", text: "Answer 1" }], stopReason: "stop" },
    { role: "user", content: "Prompt 2" },
  ];

  // Cutting at index 2 (the toolResult) is NEVER safe
  assert.equal(isToolCallSafe(history, 2), false);

  // Cutting at index 4 (the next user prompt) is safe because call_abc and its result stay together in discarded
  assert.equal(isToolCallSafe(history, 4), true);

  // Cutting at index 3 (assistant after tool result) is tool-call safe (both call and result are in discarded)
  assert.equal(isToolCallSafe(history, 3), true);

  console.log("  ✅ isToolCallSafe prevents orphan tool results and invalid cut points");
}

// 2. findCutPoint respects keepRecentTokens and selects safe user boundary
{
  // Build a 10-turn history (20 messages)
  const history: AgentMessage[] = [];
  for (let i = 1; i <= 10; i++) {
    history.push({ role: "user", content: `User query ${i} - ${"X".repeat(500)}` });
    history.push({
      role: "assistant",
      id: `a_${i}`,
      content: [{ type: "text", text: `Assistant response ${i} - ${"Y".repeat(500)}` }],
      stopReason: "stop",
    });
  }

  // Each turn is ~1,000 chars = ~250 tokens.
  // 10 turns = ~2,500 tokens.
  // If we request keepRecentTokens = 500 (~2 turns):
  const { firstKeptIndex, isUserBoundary } = findCutPoint(history, 500);

  assert.equal(isUserBoundary, true);
  assert.equal(firstKeptIndex > 0, true);
  assert.equal(history[firstKeptIndex].role, "user");
  assert.equal(firstKeptIndex % 2, 0); // User messages are at even indices

  console.log("  ✅ findCutPoint identifies clean user turn cut point preserving token budget");
}

// 3. findCutPoint never cuts at toolResult even if tokens match exactly
{
  const history: AgentMessage[] = [
    { role: "user", content: "Turn 1" },
    { role: "assistant", id: "a1", content: [{ type: "text", text: "Ans 1" }], stopReason: "stop" },
    { role: "user", content: "Turn 2" },
    {
      role: "assistant",
      id: "a2",
      content: [{ type: "toolCall", call: { id: "c1", name: "bash", arguments: { command: "ls" } } }],
      stopReason: "toolUse",
    },
    { role: "toolResult", results: [{ toolCallId: "c1", content: "file.txt" }] },
    { role: "assistant", id: "a3", content: [{ type: "text", text: "Finished" }], stopReason: "stop" },
  ];

  const { firstKeptIndex } = findCutPoint(history, 50);
  assert.notEqual(history[firstKeptIndex].role, "toolResult");
  assert.equal(isToolCallSafe(history, firstKeptIndex), true);

  console.log("  ✅ findCutPoint guarantees toolResult is never the split boundary");
}

console.log("All compaction cut-point tests passed! ✨");
