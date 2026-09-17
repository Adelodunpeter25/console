/**
 * Smol-model compaction summary tests.
 * The summarizer resolves the smol role, feeds it a bounded flat transcript,
 * and fails loudly (structural fallback) on any problem — never retries.
 */
import assert from "node:assert/strict";
import type { AgentMessage, Model } from "@console/types";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import {
  SUMMARY_INPUT_MAX_CHARS,
  buildSummaryTranscript,
  createSmolSummarizer,
  packSummaryInput,
} from "@/agent/src/compaction/llm-compaction.js";

console.log("Running smol summarizer tests...");

const mainModel: Model = { id: "big", provider: "antigravity", contextWindow: 1_000_000 };
const smolModel: Model = { id: "small", provider: "antigravity", contextWindow: 200_000 };

function textStream(text: string): StreamFn {
  return async function* () {
    yield { type: "text", text };
  };
}

// 1. Missing smol role throws before touching any transport
{
  let transportUsed = false;
  const summarize = createSmolSummarizer({
    getFallbackModel: () => mainModel,
    getStreamFn: () => {
      transportUsed = true;
      return textStream("x");
    },
    hasSmolRole: async () => false,
  });
  await assert.rejects(() => summarize([]), /No smol model configured/);
  assert.equal(transportUsed, false);
  console.log("  ✅ missing smol role throws without calling transport");
}

// 2. Summary flows through the smol model's own transport
{
  let captured: any = null;
  const summarize = createSmolSummarizer({
    getFallbackModel: () => mainModel,
    getStreamFn: (model) => {
      assert.equal(model.id, "small");
      return (async function* (params: any) {
        captured = params;
        yield { type: "text", text: "  condensed history  " };
      }) as StreamFn;
    },
    hasSmolRole: async () => true,
    resolveSmol: async () => smolModel,
  });
  const summary = await summarize([{ role: "user", content: "do the thing" }]);
  assert.equal(summary, "condensed history");
  assert.deepEqual(captured.tools, []);
  assert.equal(captured.messages.length, 1);
  assert.equal(captured.messages[0].role, "user");
  assert.ok(typeof captured.systemPrompt === "string" && captured.systemPrompt.length > 0);
  console.log("  ✅ smol transport produces the summary");
}

// 3. Oversized history is bounded to a flat transcript
{
  let captured: any = null;
  const summarize = createSmolSummarizer({
    getFallbackModel: () => mainModel,
    getStreamFn: () => (async function* (params: any) {
      captured = params;
      yield { type: "text", text: "ok" };
    }) as StreamFn,
    hasSmolRole: async () => true,
    resolveSmol: async () => smolModel,
    maxInputChars: 1_000,
  });
  const history: AgentMessage[] = [
    { role: "user", content: "original goal" },
    {
      role: "assistant",
      id: "a1",
      content: [{ type: "toolCall", call: { id: "c1", name: "read", arguments: { path: "big.ts" } } }],
      stopReason: "toolUse",
    },
    { role: "toolResult", results: [{ toolCallId: "c1", content: "z".repeat(200_000) }] },
  ];
  await summarize(history);
  const sent = JSON.stringify(captured.messages);
  assert.ok(sent.length <= 1_000 + 500, `transcript must be bounded, got ${sent.length}`);
  assert.ok(sent.includes("original goal"), "oldest context is kept");
  assert.ok(sent.includes("omitted for length"), "truncation is marked");
  assert.ok(!sent.includes("tool_use") && !sent.includes("tool_result"), "no provider wire blocks");
  console.log("  ✅ summarizer input is bounded, flat, and provider-neutral");
}

// 4. Empty summary and transport errors throw for structural fallback
{
  const empty = createSmolSummarizer({
    getFallbackModel: () => mainModel,
    getStreamFn: () => textStream("   "),
    hasSmolRole: async () => true,
    resolveSmol: async () => smolModel,
  });
  await assert.rejects(() => empty([{ role: "user", content: "hi" }]), /Empty compaction summary/);

  let calls = 0;
  const flaky = createSmolSummarizer({
    getFallbackModel: () => mainModel,
    getStreamFn: () =>
      (async function* () {
        calls++;
        throw new Error("provider down");
      }) as StreamFn,
    hasSmolRole: async () => true,
    resolveSmol: async () => smolModel,
  });
  await assert.rejects(() => flaky([{ role: "user", content: "hi" }]), /provider down/);
  assert.equal(calls, 1, "summarizer must not retry internally");
  console.log("  ✅ empty/failed summaries throw exactly once");
}

// 5. Transcript helpers handle edge shapes
{
  assert.deepEqual(buildSummaryTranscript([]), []);
  const packed = packSummaryInput([]);
  assert.equal(packed.length, 1);
  assert.ok(packed[0]!.role === "user");
  const packedCustom = packSummaryInput([{ role: "user", content: "hi" }], 10);
  assert.ok(JSON.stringify(packedCustom).length <= 200, "tiny budgets still produce a message");
  console.log("  ✅ transcript helpers handle empty and tiny budgets");
}

console.log("Smol summarizer tests passed!\n");
process.exit(0);
