/**
 * Short-session shake tests.
 * Oversized tool output on turn 1 must be shaken even though there is no
 * history to summarize — and emergency mode must reach protected turns.
 */
import assert from "node:assert/strict";
import type { AgentMessage } from "@console/types";
import { compactHistory } from "@/agent/src/compaction/index.js";
import { shakeConversation } from "@/agent/src/compaction/shake.js";

console.log("Running compaction short-session tests...");

function bigResult(chars: number): AgentMessage {
  return {
    role: "toolResult",
    results: [{ toolCallId: "c1", content: "x".repeat(chars) }],
  };
}

// 1. 2-message history with a 1.5MB tool result compacts without throwing
{
  const messages: AgentMessage[] = [
    { role: "user", content: "read everything" },
    {
      role: "assistant",
      id: "a1",
      content: [{ type: "toolCall", call: { id: "c1", name: "read", arguments: {} } }],
      stopReason: "toolUse",
    },
    bigResult(1_500_000),
  ];
  const result = compactHistory(messages, {});
  const shaken = result.compactedMessages.find((m) => m.role === "toolResult");
  assert.ok(shaken && shaken.role === "toolResult");
  const content = shaken.results[0]!.content;
  assert.ok(typeof content === "string" && content.length < 20_000);
  console.log("  ✅ short session shakes instead of throwing");
}

// 2. Non-emergency shake spares the protected suffix
{
  const messages: AgentMessage[] = [
    { role: "user", content: "old" },
    bigResult(50_000),
    { role: "user", content: "new" },
    bigResult(50_000),
  ];
  const shaken = shakeConversation(messages, 8_000, 2);
  const first = shaken[1]!;
  assert.ok(first.role === "toolResult" && (first.results[0]!.content as string).length < 20_000);
  const recent = shaken[3]!;
  assert.ok(
    recent.role === "toolResult" && (recent.results[0]!.content as string).length === 50_000,
    "protected recent turn must be untouched",
  );
  console.log("  ✅ protected suffix spared without emergency flag");
}

// 3. Emergency shake reaches everywhere, down to the emergency budget
{
  const messages: AgentMessage[] = [
    { role: "user", content: "old" },
    bigResult(50_000),
    { role: "user", content: "new" },
    bigResult(50_000),
  ];
  const shaken = shakeConversation(messages, 2_000, 2, true);
  for (const m of shaken) {
    if (m.role !== "toolResult") continue;
    assert.ok((m.results[0]!.content as string).length < 5_000);
  }
  console.log("  ✅ emergency shake truncates protected turns too");
}

console.log("Compaction short-session tests passed!\n");
process.exit(0);
