import assert from "node:assert/strict";
import { bashTool, bashJobTool } from "@/agent/src/tools/index.js";
import { BashJobManager, bashJobManager } from "@/agent/src/tools/bash/index.js";

console.log("Running BashJob background execution tests...");

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
type ToolOut = { content: Array<{ text: string }>; isError?: boolean };
const textOf = (res: unknown) => (res as ToolOut).content.map((c) => c.text).join("\n");
const extractJobId = (res: unknown): string => {
  const m = /job_[0-9a-f]{16}/.exec(textOf(res));
  assert.ok(m, `expected jobId in: ${textOf(res)}`);
  return m[0];
};

try {
  // 1. Sync behavior unchanged when background omitted
  {
    bashJobManager.resetForTests();
    const args = bashTool.inputSchema.parse({ command: "printf sync-ok" });
    const res = (await bashTool.execute(args)) as ToolOut;
    assert.ok(textOf(res).includes("sync-ok"), "sync bash still works");
    assert.equal(bashJobManager.size, 0, "sync run must not create a job");
    console.log("  ✅ sync bash unchanged");
  }

  // 2. background=true returns promptly with running jobId
  let quickJob: string;
  {
    bashJobManager.resetForTests();
    const t0 = Date.now();
    const args = bashTool.inputSchema.parse({ command: "sleep 1", background: true });
    const res = (await bashTool.execute(args)) as ToolOut;
    const dt = Date.now() - t0;
    assert.ok(dt < 1000, `background start must return promptly (took ${dt}ms)`);
    const txt = textOf(res);
    assert.ok(txt.includes("Started: "), "start must announce the job");
    assert.ok(!txt.includes("Exit code"), "start must NOT claim an exit result");
    assert.ok(txt.includes("is not an exit result"), "start must warn this isn't an exit result");
    quickJob = extractJobId(res);
    assert.equal(bashJobManager.status(quickJob).status, "running");
    console.log("  ✅ background start returns promptly with jobId");
  }

  // 3. status transitions to exited with correct exit code
  {
    const s = await bashJobManager.wait(quickJob, 5000);
    assert.equal(s.status, "exited");
    assert.equal(s.exitCode, 0);
    console.log("  ✅ status transitions running -> exited");
  }

  // 4. stdout/stderr available via output polling
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "echo out-hello; echo err-hello >&2", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);
    await bashJobManager.wait(id, 5000);
    const out = bashJobManager.output(id);
    assert.ok(out.stdout.includes("out-hello"), `stdout polling: ${out.stdout}`);
    assert.ok(out.stderr.includes("err-hello"), `stderr polling: ${out.stderr}`);
    console.log("  ✅ stdout/stderr via output polling");
  }

  // 5. Incremental polling: no duplication, cursor advances
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "echo one; echo two", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);
    await bashJobManager.wait(id, 5000);
    const p1 = bashJobManager.output(id, { cursor: 0, limit: 3 });
    assert.equal(p1.stdout, "one");
    const p2 = bashJobManager.output(id, { cursor: p1.nextCursor });
    assert.ok(p2.stdout.includes("two"), `second page: ${JSON.stringify(p2.stdout)}`);
    assert.ok(!p2.stdout.includes("one"), "must not duplicate first page");
    console.log("  ✅ incremental polling with cursor");
  }

  // 6. wait: final output for short cmd; still-running when limit expires
  {
    bashJobManager.resetForTests();
    const r1 = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "echo fast", background: true }),
    )) as ToolOut;
    const fast = extractJobId(r1);
    assert.equal((await bashJobManager.wait(fast, 5000)).status, "exited");
    const r2 = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "sleep 3", background: true }),
    )) as ToolOut;
    const slow = extractJobId(r2);
    assert.equal((await bashJobManager.wait(slow, 200)).status, "running");
    bashJobManager.kill(slow);
    console.log("  ✅ wait semantics (final vs still-running)");
  }

  // 7. kill terminates the job
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "sleep 5", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);
    await sleep(200);
    assert.equal(bashJobManager.kill(id).status, "killed");
    console.log("  ✅ kill terminates job");
  }

  // 8. background timeout -> expired
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "sleep 5", background: true, timeoutMs: 1000 }),
    )) as ToolOut;
    const id = extractJobId(res);
    await sleep(1600);
    const s = bashJobManager.status(id);
    assert.equal(s.status, "expired", `expected expired, got ${s.status}`);
    assert.equal(s.timedOut, true);
    console.log("  ✅ background timeout reports expired");
  }

  // 9. Ownership: another session cannot read or kill
  {
    const m = new BashJobManager();
    try {
      const rec = m.start({ command: "sleep 2", cwd: process.cwd(), env: process.env, timeoutMs: 30_000, ownerSessionId: "sess-a" });
      assert.throws(() => m.status(rec.jobId, "sess-b"), /not found or expired/);
      assert.throws(() => m.kill(rec.jobId, "sess-b"), /not found or expired/);
      assert.throws(() => m.output(rec.jobId, { ownerSessionId: "sess-b" }), /not found or expired/);
      assert.doesNotThrow(() => m.status(rec.jobId, "sess-a"));
      assert.equal(m.list("sess-b").length, 0);
      assert.equal(m.list("sess-a").length, 1);
    } finally {
      m.resetForTests();
    }
    console.log("  ✅ cross-session isolation");
  }

  // 10. Failed command vs failed lookup vs running are distinguishable
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "exit 3", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);
    await bashJobManager.wait(id, 5000);
    const failed = bashJobManager.status(id);
    assert.equal(failed.status, "failed");
    assert.equal(failed.exitCode, 3);
    assert.throws(() => bashJobManager.status("job_deadbeefdeadbeef"), /not found or expired/);
    const r2 = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "sleep 2", background: true }),
    )) as ToolOut;
    assert.equal(bashJobManager.status(extractJobId(r2)).status, "running");
    const miss = (await bashJobTool.execute(
      bashJobTool.inputSchema.parse({ action: "status", jobId: "job_deadbeefdeadbeef" }),
    )) as ToolOut;
    assert.equal(miss.isError, true);
    assert.ok(textOf(miss).includes("not found or expired"));
    bashJobManager.resetForTests();
    console.log("  ✅ failed vs lookup-miss vs running distinguished");
  }

  // 11. Output cap: continuous writer cannot exhaust memory
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "yes x | head -c 1000000", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);
    await bashJobManager.wait(id, 8000);
    const out = bashJobManager.output(id);
    assert.ok(out.stdout.length <= 256 * 1024 + 1, `capped at 256KB, got ${out.stdout.length}`);
    assert.equal(out.truncated, true);
    console.log("  ✅ output ring buffer caps continuous writers");
  }

  // 12. Max concurrent jobs enforced
  {
    const m = new BashJobManager();
    try {
      for (let i = 0; i < 10; i++) {
        m.start({ command: "sleep 5", cwd: process.cwd(), env: process.env, timeoutMs: 30_000 });
      }
      assert.throws(() => m.start({ command: "sleep 1", cwd: process.cwd(), env: process.env, timeoutMs: 5000 }), /Too many running/);
    } finally {
      m.resetForTests();
    }
    console.log("  ✅ concurrent-job limit enforced");
  }

  // 13. killAll cleans up running jobs (server shutdown)
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "sleep 5", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);
    bashJobManager.killAll();
    assert.equal(bashJobManager.status(id).status, "killed");
    console.log("  ✅ killAll cleans up on shutdown");
  }

  // 14. bashJob tool list/wait round-trip
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "echo tool-roundtrip", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);
    const w = (await bashJobTool.execute(bashJobTool.inputSchema.parse({ action: "wait", jobId: id, waitMs: 5000 }))) as ToolOut;
    assert.ok(textOf(w).includes("tool-roundtrip"));
    assert.ok(textOf((await bashJobTool.execute(bashJobTool.inputSchema.parse({ action: "list" }))) as ToolOut).includes(id));
    console.log("  ✅ bashJob tool round-trip");
  }

  // 15. Kill reaches grandchildren (regression for detached: true).
  {
    bashJobManager.resetForTests();
    const marker = `bg-grandchild-${process.pid}-${Date.now()}`;
    // "( sleep 30 ; echo X ) & sleep 30" — bash spawns a subshell that runs
    // sleep in the background, then runs another sleep in the foreground.
    // The background sleep is a grandchild of the top bash process.
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({
        command: `( sleep 30 ; echo ${marker} ) & sleep 30`,
        background: true,
      }),
    )) as ToolOut;
    const id = extractJobId(res);
    await sleep(400);

    const { execSync } = await import("node:child_process");
    // Both sleeps and the wrapping bash all have MARKER in their argv, so
    // pgrep -f matches them. The grandchild sleep survives without the fix.
    const countMatching = () =>
      execSync(`pgrep -f ${JSON.stringify(marker)} || true`, { encoding: "utf8" })
        .trim()
        .split("\n")
        .filter(Boolean).length;

    const before = countMatching();
    assert.ok(before >= 2, `expected >= 2 matching processes before kill, got ${before}`);

    bashJobManager.kill(id);
    await sleep(800); // SIGKILL escalation (300ms) + grace.

    const after = countMatching();
    assert.equal(after, 0, `expected 0 matching processes after kill, got ${after}`);
    console.log("  ✅ kill reaches grandchildren via process group");
  }

  // 16. wait action's snapshot header doesn't duplicate (regression for the
  // slice(5) bug). Previously, renderOutput included a snapshot header that
  // the wait handler tried to strip via `text.split("\n").slice(5)`. That
  // assumed the snapshot was always exactly 5 lines; with `finishedAt` and
  // `exitCode` present (finished jobs), it's 7, so the header reappeared
  // twice in the body.
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "echo wait-snapshot-test", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);
    const w = (await bashJobTool.execute(
      bashJobTool.inputSchema.parse({ action: "wait", jobId: id, waitMs: 5000 }),
    )) as ToolOut;
    const txt = textOf(w);
    // The buggy `slice(5)` left lines 6+ (Finished/Exit code) in the body,
    // duplicating them after the freshly-prepended snapshot header.
    const exitCodeMatches = txt.match(/^Exit code: /gm) ?? [];
    assert.equal(
      exitCodeMatches.length,
      1,
      `Exit code line must appear exactly once, got ${exitCodeMatches.length}:\n${txt}`,
    );
    const finishedMatches = txt.match(/^Finished: /gm) ?? [];
    assert.equal(
      finishedMatches.length,
      1,
      `Finished line must appear exactly once, got ${finishedMatches.length}:\n${txt}`,
    );
    assert.ok(txt.includes("Finished:"));
    assert.ok(txt.includes("Exit code: 0"));
    console.log("  ✅ wait action snapshot header not duplicated");
  }

  // 17. Stop (abort) during wait: resolves immediately, rejects as AbortError,
  // and kills the job (regression for the "stop looks dead for waitMs" bug).
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "sleep 30", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);

    const controller = new AbortController();
    const waitPromise = bashJobManager.wait(id, 60_000, undefined, controller.signal);
    await sleep(150);
    const t0 = Date.now();
    controller.abort();
    await assert.rejects(() => waitPromise, /aborted/i);
    const dt = Date.now() - t0;
    assert.ok(dt < 1000, `abort must break the wait immediately (took ${dt}ms)`);
    assert.equal(bashJobManager.status(id).status, "killed", "abort must kill the job");
    console.log("  ✅ abort during wait ends immediately and kills the job");
  }

  // 18. bashJobTool wait must NOT swallow the abort — the harness maps the
  // rejection to "cancelled by user abort" and stops the run.
  {
    bashJobManager.resetForTests();
    const res = (await bashTool.execute(
      bashTool.inputSchema.parse({ command: "sleep 30", background: true }),
    )) as ToolOut;
    const id = extractJobId(res);

    const controller = new AbortController();
    const toolPromise = bashJobTool.execute(
      bashJobTool.inputSchema.parse({ action: "wait", jobId: id, waitMs: 60_000 }),
      controller.signal,
    );
    await sleep(150);
    controller.abort();
    await assert.rejects(() => toolPromise, /aborted/i);
    assert.equal(bashJobManager.status(id).status, "killed");
    console.log("  ✅ bashJob wait tool rejects on abort instead of returning");
  }

  console.log("BashJob background execution tests passed!\n");
} finally {
  bashJobManager.resetForTests();
  bashJobManager.killAll();
}

process.exit(0);
