/**
 * Unit Tests for the OpenCode serve sidecar provider.
 * Operates offline — pure event mapping, no server needed (0 LLM credits).
 *
 * Covers:
 *  1. normalizeServeModelId → strips opencode/ prefix
 *  2. mapServeEvent → text / thinking / usage / done / error deltas
 *  3. serveTokensToUsage → TurnUsage math + hit/miss
 *  4. buildServePromptText → first turn carries system + history, later turns latest user text
 *  5. provider-registry opencode entry resolves to opencodeServeStreamFn
 *  6. live sidecar round trip (OPENCODE_SERVE_REAL=1) — skipped otherwise
 */
import assert from "node:assert/strict";
import type { AgentMessage } from "@console/types";
import { getProvider } from "@/agent/src/commands/provider-registry.js";
import {
  buildServePromptText,
  opencodeServeStreamFn,
} from "@/providers/src/opencode/serve-stream-fn.js";
import {
  mapServeEvent,
  normalizeServeModelId,
  serveTokensToUsage,
} from "@/providers/src/opencode/serve-client.js";

console.log("Running OpenCode serve sidecar provider tests...");

// 1. Model id normalization
{
  assert.equal(normalizeServeModelId("opencode/big-pickle"), "big-pickle");
  assert.equal(normalizeServeModelId("big-pickle"), "big-pickle");
  assert.equal(normalizeServeModelId("muse-spark-1.3-contributor-free"), "muse-spark-1.3-contributor-free");
  console.log("  ✅ normalizeServeModelId");
}

// 2. Event mapping (shapes captured from a live serve probe)
{
  const sid = "ses_test123";

  const text = mapServeEvent(
    { type: "session.text.delta", data: { sessionID: sid, assistantMessageID: "m1", ordinal: 0, delta: "hello" } } as never,
    sid,
  );
  assert.deepEqual(text.delta, { type: "text", text: "hello" });

  const thinking = mapServeEvent(
    { type: "session.reasoning.delta", data: { sessionID: sid, delta: "hmm" } } as never,
    sid,
  );
  assert.deepEqual(thinking.delta, { type: "thinking", text: "hmm" });

  // Foreign session events are ignored on a shared global stream.
  const foreign = mapServeEvent(
    { type: "session.text.delta", data: { sessionID: "ses_other", delta: "x" } } as never,
    sid,
  );
  assert.deepEqual(foreign, {});

  // Unknown types (heartbeats, inbox, instructions) are ignored.
  assert.deepEqual(mapServeEvent({ type: "server.connected", data: {} } as never, sid), {});
  assert.deepEqual(mapServeEvent({ type: "session.text.delta", data: { sessionID: sid } } as never, sid), {});

  const done = mapServeEvent({ type: "session.execution.succeeded", data: { sessionID: sid } } as never, sid);
  assert.equal(done.done, true);

  const failed = mapServeEvent(
    { type: "session.execution.failed", data: { sessionID: sid, error: { message: "boom" } } } as never,
    sid,
  );
  assert.equal(failed.error, "boom");

  console.log("  ✅ mapServeEvent");
}

// 3. Usage math
{
  const hit = serveTokensToUsage({ input: 100, output: 10, reasoning: 5, cache: { read: 40, write: 2 } });
  assert.equal(hit.input, 60);
  assert.equal(hit.cacheRead, 40);
  assert.equal(hit.cacheWrite, 2);
  assert.equal(hit.output, 10);
  assert.equal(hit.reasoningTokens, 5);
  assert.equal(hit.totalTokens, 115);
  assert.equal(hit.cacheStatus, "hit");

  const miss = serveTokensToUsage({ input: 50, output: 4 });
  assert.equal(miss.input, 50);
  assert.equal(miss.cacheStatus, "miss");

  const empty = serveTokensToUsage(undefined);
  assert.equal(empty.cacheStatus, "miss");
  assert.equal(empty.totalTokens, 0);

  console.log("  ✅ serveTokensToUsage");
}

// 4. Prompt text building
{
  const messages: AgentMessage[] = [
    { role: "user", content: "List files" },
    {
      role: "assistant",
      id: "turn-1",
      content: [{ type: "text", text: "Here they are." }],
      stopReason: "stop",
    },
    { role: "user", content: "Now edit a.ts" },
  ];

  const first = buildServePromptText("SYS", messages, true);
  assert.ok(first.startsWith("SYS"));
  assert.ok(first.includes("User: List files"));
  assert.ok(first.includes("Assistant: Here they are."));
  assert.ok(first.includes("User: Now edit a.ts"));

  const later = buildServePromptText("SYS", messages, false);
  assert.equal(later, "Now edit a.ts");

  console.log("  ✅ buildServePromptText");
}

// 5. Registry wiring
{
  const entry = getProvider("opencode");
  assert.ok(entry, "opencode provider must exist");
  assert.equal(entry!.getStreamFn(), opencodeServeStreamFn);
  console.log("  ✅ provider-registry opencode → opencodeServeStreamFn");
}

// 6. Live sidecar round trip — only with OPENCODE_SERVE_REAL=1 and a sidecar
// up (console start). Never runs in CI by default.
if (process.env.OPENCODE_SERVE_REAL === "1") {
  const { loadSidecarRef } = await import("@/providers/src/opencode/serve-client.js");
  const ref = await loadSidecarRef();
  if (!ref) {
    console.log("  ⚠️  OPENCODE_SERVE_REAL=1 but no sidecar state — skipping live test");
  } else {
    const model = { id: "big-pickle", provider: "opencode", contextWindow: 200_000 } as const;
    const deltas: unknown[] = [];
    for await (const d of opencodeServeStreamFn({
      model,
      systemPrompt: "You are a test assistant.",
      messages: [{ role: "user", content: "reply with exactly: hello serve" }],
      tools: [],
      cacheIdentity: { conversationId: `conv-test-${Date.now()}` },
    })) {
      deltas.push(d);
    }
    const texts = deltas.filter((d): d is { type: "text"; text: string } => (d as { type: string }).type === "text");
    const usages = deltas.filter((d): d is { type: "usage"; usage: unknown } => (d as { type: string }).type === "usage");
    assert.ok(texts.length > 0, "expected text deltas");
    assert.equal(usages.length, 1, "expected exactly one usage delta");
    assert.ok(texts.map((t) => t.text).join("").includes("hello serve"));
    console.log("  ✅ live sidecar round trip (big-pickle)");
  }
} else {
  console.log("  ⏭️  live sidecar test skipped (set OPENCODE_SERVE_REAL=1 with a sidecar up)");
}

console.log("OpenCode serve sidecar provider tests passed!\n");
