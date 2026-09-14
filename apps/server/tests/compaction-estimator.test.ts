/**
 * Payload token estimator tests (P0-B3).
 * Verifies the wire-payload estimate covers system prompt, tool schemas, and
 * dense tool output — the gaps in the old message-only heuristic.
 */
import assert from "node:assert/strict";
import type { AgentMessage } from "@console/types";
import { estimateMessageTokens, estimatePayloadTokens } from "@/agent/src/compaction/token-estimator.js";
import { shouldCompact } from "@/agent/src/compaction/index.js";

console.log("Running compaction estimator tests...");

const testModel = { id: "m", provider: "antigravity", contextWindow: 10_000 } as const;

// 1. System prompt and tools are counted on top of messages
{
  const messages: AgentMessage[] = [{ role: "user", content: "hi" }];
  const base = estimatePayloadTokens({ messages });
  const full = estimatePayloadTokens({
    messages,
    systemPrompt: "x".repeat(40_000),
    tools: Array.from({ length: 10 }, (_, i) => ({ name: `tool${i}`, description: "does things" })),
  });
  assert.ok(full.tokens > base.tokens + 10_000, `system+tools must add >10k tokens, got ${full.tokens - base.tokens}`);
  assert.equal(full.source, "local");
  console.log("  ✅ system prompt + tool schemas counted");
}

// 2. Code/JSON/tool output counts denser than prose
{
  const blob = "x".repeat(3000);
  const asResult = estimatePayloadTokens({
    messages: [{ role: "toolResult", results: [{ toolCallId: "c1", content: blob }] }],
    wireMarginTokens: 0,
  });
  const asProse = estimatePayloadTokens({
    messages: [{ role: "user", content: blob }],
    wireMarginTokens: 0,
  });
  assert.equal(asResult.tokens, 1000);
  assert.equal(asProse.tokens, 750);
  console.log("  ✅ dense tool output uses code density");
}

// 3. Wire margin defaults to 2000 and is configurable
{
  assert.equal(estimatePayloadTokens({ messages: [] }).tokens, 2000);
  assert.equal(estimatePayloadTokens({ messages: [], wireMarginTokens: 0 }).tokens, 0);
  console.log("  ✅ wire margin default + override");
}

// 4. shouldCompact with payload triggers before messages-only does
{
  const messages: AgentMessage[] = [{ role: "user", content: "x".repeat(1000) }];
  assert.equal(shouldCompact(messages, { ...testModel }), false);
  assert.equal(
    shouldCompact(messages, { ...testModel }, {}, { systemPrompt: "y".repeat(40_000) }),
    true,
  );
  console.log("  ✅ payload-aware shouldCompact catches system-heavy payloads");
}

// 5. Message-only path is unchanged (backward compatible)
{
  const messages: AgentMessage[] = [{ role: "user", content: "abcd" }];
  assert.equal(estimateMessageTokens(messages), 1);
  assert.equal(shouldCompact(messages, { ...testModel }), false);
  console.log("  ✅ legacy message-only estimation intact");
}

console.log("Compaction estimator tests passed!\n");
process.exit(0);
