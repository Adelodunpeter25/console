import assert from "node:assert/strict";
import { repairToolCallHistory } from "../agent/src/utils/tool-history.js";
import type { AgentMessage } from "@console/types";

console.log("Running tool history repair tests...");

const assistantWithTwoCalls: AgentMessage = {
  role: "assistant",
  id: "assistant-1",
  content: [
    {
      type: "toolCall",
      call: { id: "call-1", name: "writeFile", arguments: { path: "a" } },
    },
    {
      type: "toolCall",
      call: { id: "call-2", name: "writeFile", arguments: { path: "b" } },
    },
  ],
  stopReason: "toolUse",
};

// An interrupted run can leave the next user prompt directly after tool use.
const missing = repairToolCallHistory([
  { role: "user", content: "write files" },
  assistantWithTwoCalls,
  { role: "user", content: "try again" },
]);
assert.equal(missing.repaired, true);
assert.equal(missing.messages[2]?.role, "toolResult");
if (missing.messages[2]?.role === "toolResult") {
  assert.deepEqual(
    missing.messages[2].results.map((result) => result.toolCallId),
    ["call-1", "call-2"],
  );
  assert.equal(missing.messages[2].results[0]?.isError, true);
}
assert.equal(missing.messages[3]?.role, "user");

// A partial result is repaired in-place immediately after the tool-use turn.
const partial = repairToolCallHistory([
  assistantWithTwoCalls,
  { role: "toolResult", results: [{ toolCallId: "call-1", content: "ok" }] },
]);
assert.equal(partial.repaired, true);
assert.equal(partial.messages.length, 2);
if (partial.messages[1]?.role === "toolResult") {
  assert.equal(partial.messages[1].results.length, 2);
  assert.equal(partial.messages[1].results[1]?.toolCallId, "call-2");
}

const valid = repairToolCallHistory([
  assistantWithTwoCalls,
  {
    role: "toolResult",
    results: [
      { toolCallId: "call-1", content: "ok" },
      { toolCallId: "call-2", content: "ok" },
    ],
  },
]);
assert.equal(valid.repaired, false);
assert.equal(valid.messages.length, 2);
assert.equal(valid.messages[0], assistantWithTwoCalls);

console.log("Tool history repair tests passed!\n");
