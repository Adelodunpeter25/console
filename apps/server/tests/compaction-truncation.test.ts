import assert from "node:assert/strict";
import {
  truncateHeadTail,
  truncateToolResultContent,
  truncateMessageToolResults,
} from "@/agent/src/utils/text-truncate.js";
import { estimateMessageTokens } from "@/agent/src/compaction/token-estimator.js";
import type { AgentMessage } from "@console/types";

console.log("Running compaction truncation tests...");

// 1. truncateHeadTail basic behavior
{
  const shortText = "hello world";
  assert.equal(truncateHeadTail(shortText, 100), shortText);

  // 10,000 char string with max 1,000 chars
  const longText = "A".repeat(5000) + "B".repeat(5000);
  const truncated = truncateHeadTail(longText, 1000);
  assert.equal(truncated.length < longText.length, true);
  assert.equal(truncated.startsWith("A".repeat(500)), true);
  assert.equal(truncated.endsWith("B".repeat(500)), true);
  assert.equal(truncated.includes("[... Tool output truncated: 9,000 characters elided ...]"), true);
  console.log("  ✅ truncateHeadTail preserves head and tail with elision marker");
}

// 2. truncateToolResultContent on strings, arrays, and objects
{
  const bigOutput = "LINE\n".repeat(5000); // 25,000 chars
  const res = truncateToolResultContent(bigOutput, 2000) as string;
  assert.equal(res.length < bigOutput.length, true);
  assert.equal(res.includes("Tool output truncated"), true);

  // Array of text parts
  const parts = [
    { type: "text", text: "X".repeat(10000) },
    { type: "text", text: "Y".repeat(10000) },
  ];
  const truncatedParts = truncateToolResultContent(parts, 4000) as Array<{ type: string; text: string }>;
  assert.equal(truncatedParts.length, 2);
  assert.equal(truncatedParts[0].text.includes("Tool output truncated"), true);

  console.log("  ✅ truncateToolResultContent handles strings and content part arrays");
}

// 3. truncateMessageToolResults on AgentMessage
{
  const msg: AgentMessage = {
    role: "toolResult",
    results: [
      {
        toolCallId: "call_1",
        content: "Z".repeat(50_000),
      },
      {
        toolCallId: "call_2",
        content: "Short result",
      },
    ],
  };

  const truncatedMsg = truncateMessageToolResults(msg, 2000);
  assert.equal(truncatedMsg.role, "toolResult");
  if (truncatedMsg.role === "toolResult") {
    assert.equal(typeof truncatedMsg.results[0].content === "string", true);
    assert.equal((truncatedMsg.results[0].content as string).includes("Tool output truncated"), true);
    assert.equal(truncatedMsg.results[1].content, "Short result");
  }

  // Token estimate drops drastically after truncation
  const tokensBefore = estimateMessageTokens([msg]);
  const tokensAfter = estimateMessageTokens([truncatedMsg]);
  assert.equal(tokensBefore > 12_000, true);
  assert.equal(tokensAfter < 1_000, true);

  console.log("  ✅ truncateMessageToolResults bounds oversized tool results in AgentMessage");
}

console.log("All compaction truncation tests passed! ✨");
