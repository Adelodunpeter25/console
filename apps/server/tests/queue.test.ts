/**
 * Functional tests for prompt queueing & steering (RunService + routes).
 * Runs 100% offline — the "antigravity" provider's stream fn is swapped for
 * a scripted mock so no LLM calls happen (0 credits used).
 */
import assert from "node:assert/strict";
import { createApiApp } from "@/api/src/index.js";
import { RunService } from "@/api/src/services/run.service.js";
import { PROVIDER_CATALOG } from "@/agent/src/commands/provider-registry.js";
import type { StreamFn } from "@/agent/src/index.js";

console.log("Running prompt queueing & steering tests...");

/** Swap the antigravity provider's stream fn for a scripted mock for one test block. */
function withMockStream(streamFn: StreamFn, fn: () => Promise<void>): Promise<void> {
  const original = PROVIDER_CATALOG.antigravity.getStreamFn;
  PROVIDER_CATALOG.antigravity.getStreamFn = () => streamFn;
  return fn().finally(() => {
    PROVIDER_CATALOG.antigravity.getStreamFn = original;
  });
}

const app = createApiApp();

async function post(path: string, body: unknown) {
  const res = await app.request(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return { status: res.status, json: (await res.json()) as any };
}

/** Create a session via the real route so the queue tables have a row to attach to. */
async function createSession(): Promise<string> {
  const res = await post("/api/sessions", {
    title: "Queue Test Session",
    cwd: process.cwd(),
    modelId: "gemini-3.1-pro-high",
    provider: "antigravity",
  });
  return res.json.data.id as string;
}

// 1. Queue → Get → Delete via REST routes (no active run required).
{
  const sessionId = await createSession();
  const queueRes = await post(`/api/sessions/${sessionId}/queue`, { prompt: "follow up" });
  assert.equal(queueRes.status, 200);
  assert.equal(queueRes.json.success, true);
  assert.equal(queueRes.json.data.prompt, "follow up");
  assert.equal(queueRes.json.data.sessionId, sessionId);

  const getRes = await app.request(`/api/sessions/${sessionId}/queue`);
  assert.equal(getRes.status, 200);
  const getJson = (await getRes.json()) as any;
  assert.equal(getJson.data.prompt, "follow up");

  const delRes = await app.request(`/api/sessions/${sessionId}/queue`, { method: "DELETE" });
  assert.equal(delRes.status, 200);
  const delJson = (await delRes.json()) as any;
  assert.equal(delJson.data.deleted, true);

  const getAfterDelete = await app.request(`/api/sessions/${sessionId}/queue`);
  const getAfterDeleteJson = (await getAfterDelete.json()) as any;
  assert.equal(getAfterDeleteJson.data, null);
  console.log("  ✅ POST/GET/DELETE /queue round-trip");
}

// 2. Queueing replaces a previous queued prompt (last write wins).
{
  const sessionId = await createSession();
  await post(`/api/sessions/${sessionId}/queue`, { prompt: "first" });
  const second = await post(`/api/sessions/${sessionId}/queue`, { prompt: "second" });
  assert.equal(second.json.data.prompt, "second");

  const getRes = await app.request(`/api/sessions/${sessionId}/queue`);
  const getJson = (await getRes.json()) as any;
  assert.equal(getJson.data.prompt, "second");
  console.log("  ✅ queueing a second prompt replaces the first");
}

// 3. Empty prompt is rejected.
{
  const sessionId = await createSession();
  const res = await post(`/api/sessions/${sessionId}/queue`, { prompt: "   " });
  assert.equal(res.status, 400);
  console.log("  ✅ POST /queue rejects an empty prompt");
}

// 4. /steer with no active run returns 404 and does not queue.
{
  const sessionId = await createSession();
  const res = await post(`/api/sessions/${sessionId}/steer`, { prompt: "steer me" });
  assert.equal(res.status, 404);

  const getRes = await app.request(`/api/sessions/${sessionId}/queue`);
  const getJson = (await getRes.json()) as any;
  assert.equal(getJson.data, null, "steer must not leave a queued prompt when there was no active run");
  console.log("  ✅ POST /steer 404s with no active run and leaves no queued prompt");
}

// 5. Auto-drain: a queued prompt starts automatically once the active run settles.
await withMockStream(
  async function* () {
    yield { type: "text", text: "turn one" };
  },
  async () => {
    const sessionId = crypto.randomUUID();
    const runService = new RunService();
    const events: string[] = [];
    const prompts: string[] = [];

    const runPromise = runService.runAgentStream(
      sessionId,
      { prompt: "turn one prompt", provider: "antigravity", modelId: "gemini-3.1-pro-high" },
      (event) => {
        events.push(event.type);
        if (event.type === "turnStart") prompts.push(event.prompt);
      },
    );

    // Queue a follow-up while turn one is still in flight.
    // Poll briefly for the run to register as active (mock stream resolves fast).
    for (let i = 0; i < 50 && !RunService.isRunActive(sessionId); i++) {
      await new Promise((r) => setTimeout(r, 2));
    }
    assert.ok(RunService.isRunActive(sessionId), "turn one should be active before queueing");

    const queued = runService.queuePrompt(sessionId, {
      prompt: "turn two prompt",
      provider: "antigravity",
      modelId: "gemini-3.1-pro-high",
    });
    assert.ok(queued, "queued prompt should be returned");
    assert.equal(queued.prompt, "turn two prompt");

    await runPromise;
    // Turn two is fire-and-forget from the finally block; wait for it to settle too.
    for (let i = 0; i < 100 && prompts.length < 2; i++) {
      await new Promise((r) => setTimeout(r, 5));
    }

    assert.deepEqual(prompts, ["turn one prompt", "turn two prompt"]);
    assert.ok(events.includes("queueUpdated"), "queueUpdated broadcast for the queue + the auto-clear");
    assert.equal(runService.getQueuedPrompt(sessionId), null, "queue is cleared once drained");
    console.log("  ✅ queued prompt auto-drains as turn two after turn one settles");
  },
);

// 6. Steer: aborts the active run and drains the steered prompt as the next turn.
await withMockStream(
  async function* () {
    yield { type: "text", text: "long turn" };
  },
  async () => {
    const sessionId = crypto.randomUUID();
    const runService = new RunService();
    const prompts: string[] = [];

    // A stream fn whose first turn hangs until aborted, so steer has time to act.
    const hangingStreamFn: StreamFn = async function* (params) {
      if (!params.signal?.aborted) {
        await new Promise<void>((resolve) => {
          params.signal?.addEventListener("abort", () => resolve());
        });
      }
      yield { type: "text", text: "unreachable" };
    };
    const original = PROVIDER_CATALOG.antigravity.getStreamFn;
    PROVIDER_CATALOG.antigravity.getStreamFn = () => hangingStreamFn;

    try {
      const runPromise = runService.runAgentStream(
        sessionId,
        { prompt: "turn one prompt", provider: "antigravity", modelId: "gemini-3.1-pro-high" },
        (event) => {
          if (event.type === "turnStart") prompts.push(event.prompt);
        },
      );

      for (let i = 0; i < 50 && !RunService.isRunActive(sessionId); i++) {
        await new Promise((r) => setTimeout(r, 2));
      }
      assert.ok(RunService.isRunActive(sessionId));

      // Switch the mock back to the fast one for turn two before steering.
      const fastStreamFn: StreamFn = async function* () {
        yield { type: "text", text: "turn two" };
      };
      PROVIDER_CATALOG.antigravity.getStreamFn = () => fastStreamFn;

      const steerRes = await post(`/api/sessions/${sessionId}/steer`, {
        prompt: "steered prompt",
        provider: "antigravity",
        modelId: "gemini-3.1-pro-high",
      });
      // The route uses its own RunService instance, but activeRuns/pendingNextTurn
      // are static, so this affects the same session.
      assert.equal(steerRes.status, 200);
      assert.equal(steerRes.json.data.steered, true);

      await runPromise;
      for (let i = 0; i < 100 && prompts.length < 2; i++) {
        await new Promise((r) => setTimeout(r, 5));
      }

      assert.deepEqual(prompts, ["turn one prompt", "steered prompt"]);
      assert.equal(runService.getQueuedPrompt(sessionId), null);
      console.log("  ✅ steer aborts the active run and drains the steered prompt as turn two");
    } finally {
      PROVIDER_CATALOG.antigravity.getStreamFn = original;
    }
  },
);

// 7. PUT /queue edits the staged prompt in place (same id), 404s when empty.
{
  const sessionId = await createSession();
  const queued = await post(`/api/sessions/${sessionId}/queue`, { prompt: "original" });
  assert.equal(queued.status, 200);
  const originalId = queued.json.data.id as string;

  const putRes = await app.request(`/api/sessions/${sessionId}/queue`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt: "edited" }),
  });
  assert.equal(putRes.status, 200);
  const putJson = (await putRes.json()) as any;
  assert.equal(putJson.success, true);
  assert.equal(putJson.data.prompt, "edited");
  assert.equal(putJson.data.id, originalId, "edit must keep the queued prompt id");

  const getRes = await app.request(`/api/sessions/${sessionId}/queue`);
  const getJson = (await getRes.json()) as any;
  assert.equal(getJson.data.prompt, "edited");
  assert.equal(getJson.data.id, originalId);

  const emptyEdit = await app.request(`/api/sessions/${sessionId}/queue`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt: "   " }),
  });
  assert.equal(emptyEdit.status, 400);

  const freshId = await createSession();
  const missing = await app.request(`/api/sessions/${freshId}/queue`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt: "nothing staged" }),
  });
  assert.equal(missing.status, 404);
  console.log("  ✅ PUT /queue edits in place and 404s when nothing is staged");
}

// 8. POST /abort discards a staged prompt so Stop means stop everything.
await withMockStream(
  async function* () {
    yield { type: "text", text: "long turn" };
  },
  async () => {
    const sessionId = crypto.randomUUID();
    const runService = new RunService();
    const prompts: string[] = [];

    const hangingStreamFn: StreamFn = async function* (params) {
      if (!params.signal?.aborted) {
        await new Promise<void>((resolve) => {
          params.signal?.addEventListener("abort", () => resolve());
        });
      }
      yield { type: "text", text: "unreachable" };
    };
    const original = PROVIDER_CATALOG.antigravity.getStreamFn;
    PROVIDER_CATALOG.antigravity.getStreamFn = () => hangingStreamFn;

    try {
      const runPromise = runService.runAgentStream(
        sessionId,
        { prompt: "turn one prompt", provider: "antigravity", modelId: "gemini-3.1-pro-high" },
        (event) => {
          if (event.type === "turnStart") prompts.push(event.prompt);
        },
      );

      for (let i = 0; i < 50 && !RunService.isRunActive(sessionId); i++) {
        await new Promise((r) => setTimeout(r, 2));
      }
      assert.ok(RunService.isRunActive(sessionId));

      runService.queuePrompt(sessionId, {
        prompt: "queued follow-up",
        provider: "antigravity",
        modelId: "gemini-3.1-pro-high",
      });

      const abortRes = await post(`/api/sessions/${sessionId}/abort`, {});
      assert.equal(abortRes.status, 200);

      await runPromise;
      // Give any wrongly-drained turn two a chance to start.
      await new Promise((r) => setTimeout(r, 100));

      assert.deepEqual(prompts, ["turn one prompt"], "aborted run must not auto-start the queued prompt");
      assert.equal(runService.getQueuedPrompt(sessionId), null, "abort must clear the staged prompt");
      console.log("  ✅ POST /abort discards the staged prompt");
    } finally {
      PROVIDER_CATALOG.antigravity.getStreamFn = original;
    }
  },
);

console.log("Prompt queueing & steering tests passed!\n");
