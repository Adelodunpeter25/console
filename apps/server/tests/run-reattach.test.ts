/**
 * Tests for run re-attach: RunEventHub fan-out semantics and the
 * GET /api/sessions/:id/run/stream attach route.
 * Runs fully offline — no LLM calls.
 */
import assert from "node:assert/strict";
import { createApiApp } from "@/api/src/index.js";
import {
  RunEventHub,
  MAX_REPLAY_BUFFER,
} from "@console/types";
import type { RunStreamSubscriber, AgentSessionEvent } from "@console/types";

console.log("Running run re-attach (hub + attach route) tests...");

/** Collect delivered [seq, event.type] pairs into an array. */
function collector(log: [number, string][]): RunStreamSubscriber {
  return {
    id: "collector",
    deliver: async (seq, event) => {
      log.push([seq, event.type]);
    },
  };
}

const textPart = (text: string): AgentSessionEvent => ({
  type: "modelStreamPart",
  part: { text },
});

{
  const hub = new RunEventHub();
  try {
    // Live delivery preserves order and assigns monotonic seqs.
    const log: [number, string][] = [];
    hub.subscribe(collector(log));

    hub.broadcast({ type: "sessionStart" });
    hub.broadcast(textPart("hello "));
    hub.broadcast(textPart("world"));
    await new Promise((r) => setTimeout(r, 10));

    assert.deepEqual(
      log.map(([s]) => s),
      [1, 2, 3],
    );
    assert.deepEqual(
      log.map(([, t]) => t),
      ["sessionStart", "modelStreamPart", "modelStreamPart"],
    );
    console.log("  ✅ live delivery is ordered with monotonic seq");
  } finally {
    hub.destroy();
  }
}

{
  const hub = new RunEventHub();
  try {
    // Replay: events emitted before attach are buffered; since filters them.
    hub.broadcast({ type: "sessionStart" });
    hub.broadcast({ type: "turnStart", prompt: "hi" });
    hub.broadcast(textPart("delta"));

    const log: [number, string][] = [];
    hub.subscribe(collector(log), /* since */ 1);
    await new Promise((r) => setTimeout(r, 10));

    assert.deepEqual(
      log.map(([s]) => s),
      [2, 3],
      "only events newer than since are replayed",
    );

    // Full catch-up: since=0 delivers everything buffered.
    const log2: [number, string][] = [];
    hub.subscribe({ ...collector(log2), id: "collector-2" }, 0);
    await new Promise((r) => setTimeout(r, 10));
    assert.equal(log2.length, 3);
    console.log("  ✅ replay honors since filter and full catch-up");
  } finally {
    hub.destroy();
  }
}

{
  const hub = new RunEventHub();
  try {
    // No-gap guarantee: subscribe() snapshots the buffer synchronously, so an
    // event broadcast immediately after subscribe() is either replayed or
    // delivered live — never lost, never duplicated.
    hub.broadcast({ type: "sessionStart" });
    const log: [number, string][] = [];
    hub.subscribe(collector(log), 0);
    hub.broadcast({ type: "turnEnd", turnId: "t1" });
    await new Promise((r) => setTimeout(r, 10));

    const seqs = log.map(([s]) => s);
    assert.deepEqual(seqs, [...new Set(seqs)], "no duplicate seqs");
    assert.ok(seqs.includes(1) && seqs.includes(2), "no missing seqs");
    console.log("  ✅ attach has no gap/duplication window");
  } finally {
    hub.destroy();
  }
}

{
  const hub = new RunEventHub();
  try {
    // modelStreamPart deltas coalesce into one accumulating snapshot entry;
    // toolCall previews stay discrete.
    hub.broadcast(textPart("a"));
    hub.broadcast(textPart("b"));
    hub.broadcast({
      type: "modelStreamPart",
      part: { text: "", toolCall: { id: "tc1", name: "bash" } },
    });
    hub.broadcast(textPart("c"));

    const buffered = hub
      .bufferedSince(0)
      .filter((e): e is { seq: number; event: AgentSessionEvent; merged?: boolean } => e.event.type === "modelStreamPart");
    assert.equal(buffered.length, 3, "deltas coalesce, preview frame separate");

    const merged = buffered.find((e) => e.merged)!;
    if (merged.event.type !== "modelStreamPart") throw new Error("unreachable");
    assert.equal(merged.event.part.text, "ab");

    // Late joiner receives the accumulated snapshot once.
    const log: [number, string][] = [];
    hub.subscribe(collector(log), 0);
    await new Promise((r) => setTimeout(r, 10));
    const texts = log.filter(([, t]) => t === "modelStreamPart");
    assert.equal(texts.length, 3);
    console.log("  ✅ streaming deltas coalesce in the replay buffer");
  } finally {
    hub.destroy();
  }
}

{
  const hub = new RunEventHub();
  try {
    // Buffer is capped.
    for (let i = 0; i < MAX_REPLAY_BUFFER + 50; i++) {
      hub.broadcast({ type: "turnStart", prompt: String(i) });
    }
    assert.equal(hub.bufferedSince(0).length, MAX_REPLAY_BUFFER);
    console.log("  ✅ replay buffer respects its cap");
  } finally {
    hub.destroy();
  }
}

{
  const hub = new RunEventHub();
  try {
    // Synthetic terminal frames only reach extendedFrames subscribers.
    const legacyLog: [number, string][] = [];
    const extendedLog: [number, string][] = [];
    hub.subscribe({ ...collector(legacyLog), id: "legacy" });
    hub.subscribe({ ...collector(extendedLog), id: "ext", extendedFrames: true });

    hub.broadcast(textPart("x"));
    hub.broadcast({ type: "done" });
    await new Promise((r) => setTimeout(r, 10));

    assert.ok(!legacyLog.some(([, t]) => t === "done"), "legacy pipe gets no done frame");
    assert.ok(extendedLog.some(([, t]) => t === "done"), "extendedFrames gets done frame");
    console.log("  ✅ terminal frames only to opt-in subscribers");
  } finally {
    hub.destroy();
  }
}

{
  const hub = new RunEventHub();
  try {
    // A throwing deliver detaches the subscriber; later broadcasts skip it
    // and other subscribers keep receiving.
    const badLog: [number, string][] = [];
    const goodLog: [number, string][] = [];
    hub.subscribe({
      id: "bad",
      deliver: async (seq, event) => {
        if (event.type === "turnStart") throw new Error("socket died");
        badLog.push([seq, event.type]);
      },
    });
    hub.subscribe({ ...collector(goodLog), id: "good" });

    hub.broadcast({ type: "sessionStart" }); // both ok
    hub.broadcast({ type: "turnStart", prompt: "p" }); // bad throws
    await new Promise((r) => setTimeout(r, 10));
    hub.broadcast({ type: "turnEnd", turnId: "t" }); // only good
    await new Promise((r) => setTimeout(r, 10));

    assert.equal(hub.subscriberCount, 1);
    assert.ok(!badLog.some(([s]) => s === 2), "bad subscriber got nothing after failure");
    assert.ok(goodLog.some(([, t]) => t === "turnEnd"), "good subscriber unaffected");
    console.log("  ✅ failed delivery detaches only the failing subscriber");
  } finally {
    hub.destroy();
  }
}

{
  const hub = new RunEventHub();
  try {
    // waitForSettle-style lifecycle via destroy().
    let settled = false;
    void hub.settled.then(() => {
      settled = true;
    });
    hub.destroy();
    await Promise.resolve();
    assert.ok(settled, "destroy settles the hub");
    console.log("  ✅ hub destroy settles waiters");
  } finally {
    // already destroyed
  }
}

// --- Attach route ---

const app = createApiApp();

{
  const res = await app.request("/api/sessions/no-such-session/run/stream");
  assert.equal(res.status, 409);
  const json = (await res.json()) as { success: boolean };
  assert.equal(json.success, false);
  console.log("  ✅ GET /run/stream returns 409 when no run is active");
}

{
  const res = await app.request("/api/sessions/x/run/stream?since=-3");
  assert.equal(res.status, 400);
  console.log("  ✅ GET /run/stream rejects invalid since values");
}

console.log("✅ run re-attach tests passed\n");
