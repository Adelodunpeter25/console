import assert from "node:assert/strict";
import { TerminalPtyManager } from "@/api/src/terminal/pty.manager.js";

/**
 * PTY speed baseline (run before + after each optimization phase):
 *   cd apps/server && bun tests/pty-speed-test.ts
 *
 * Prints [pty-speed] lines to copy into notes. Fails non-zero when the
 * terminal pipeline itself is broken (no spawn / no echo / no flood bytes).
 */
async function timeSpawnNoResize(mgr: TerminalPtyManager): Promise<number> {
  const t0 = performance.now();
  const { id, ready } = mgr.spawn({ cwd: "/tmp", shell: "/bin/sh", cols: 120, rows: 40 });
  mgr.attach(id, { onData: () => {}, onExit: () => {}, onError: () => {} });
  await ready;
  const ms = performance.now() - t0;
  mgr.kill(id);
  return ms;
}

async function timeSpawnWithResize(mgr: TerminalPtyManager): Promise<number> {
  const t0 = performance.now();
  const { id, ready } = mgr.spawn({ cwd: "/tmp", shell: "/bin/sh", cols: 120, rows: 40 });
  mgr.attach(id, { onData: () => {}, onExit: () => {}, onError: () => {} });
  mgr.resize(id, 101, 31);
  await ready;
  const ms = performance.now() - t0;
  mgr.kill(id);
  return ms;
}

async function timeEcho(mgr: TerminalPtyManager): Promise<number> {
  const { id, ready } = mgr.spawn({ cwd: "/tmp", shell: "/bin/sh", cols: 120, rows: 40 });
  let resolveEcho!: () => void;
  const echoSeen = new Promise<void>((r) => (resolveEcho = r));
  mgr.attach(id, {
    onData: (chunk) => {
      // NOTE: the PTY echoes typed input, so the sentinel must NOT appear in
      // the command line itself — $((11*13)) evaluates to 143 only in output.
      if (new TextDecoder().decode(chunk).includes("OUT-143")) resolveEcho();
    },
    onExit: () => {},
    onError: () => {},
  });
  mgr.resize(id, 120, 40);
  await ready;
  const t0 = performance.now();
  mgr.write(id, "echo OUT-$((11*13))\r");
  await Promise.race([
    echoSeen,
    new Promise((_, rej) => setTimeout(() => rej(new Error("echo timeout")), 8000)),
  ]);
  const ms = performance.now() - t0;
  mgr.kill(id);
  return ms;
}

async function flood(mgr: TerminalPtyManager): Promise<{ frames: number; bytes: number; ms: number }> {
  const { id, ready } = mgr.spawn({ cwd: "/tmp", shell: "/bin/sh", cols: 120, rows: 40 });
  let frames = 0;
  let bytes = 0;
  let resolveDone!: () => void;
  const done = new Promise<void>((r) => (resolveDone = r));
  mgr.attach(id, {
    onData: (chunk) => {
      frames++;
      bytes += chunk.byteLength;
      if (new TextDecoder().decode(chunk).includes("DONE-42")) resolveDone();
    },
    onExit: () => {},
    onError: () => {},
  });
  mgr.resize(id, 120, 40);
  await ready;
  const t0 = performance.now();
  // NOTE: sentinel is computed ($((6*7)) -> 42) so the PTY's echo of the typed
  // line itself can never match — only true command output resolves `done`.
  // Output is in-order, so everything before the sentinel has arrived by then.
  mgr.write(id, "yes FLOODLINE | head -n 2000; echo DONE-$((6*7))\r");
  await Promise.race([
    done,
    new Promise((_, rej) => setTimeout(() => rej(new Error("flood timeout")), 15000)),
  ]);
  const ms = performance.now() - t0;
  mgr.kill(id);
  return { frames, bytes, ms };
}

async function main(): Promise<void> {
  const mgr = new TerminalPtyManager();

  const noResize = await timeSpawnNoResize(mgr);
  const withResize = await timeSpawnWithResize(mgr);
  const echo = await timeEcho(mgr);
  const fl = await flood(mgr);
  const fps = fl.ms > 0 ? Math.round((fl.frames / fl.ms) * 1000) : 0;

  console.log(`[pty-speed] spawn-no-resize-ms=${Math.round(noResize)}`);
  console.log(`[pty-speed] spawn-with-resize-ms=${Math.round(withResize)}`);
  console.log(`[pty-speed] echo-roundtrip-ms=${Math.round(echo)}`);
  console.log(
    `[pty-speed] flood-frames=${fl.frames} flood-bytes=${fl.bytes} flood-ms=${Math.round(fl.ms)} frames-per-sec=${fps}`,
  );

  assert.ok(noResize < 15000, "spawn without resize took too long");
  assert.ok(withResize < 15000, "spawn with resize took too long");
  assert.ok(fl.bytes > 5000, "flood produced almost no bytes — pipeline broken?");
  assert.ok(fl.frames > 0, "flood produced no frames — pipeline broken?");

  mgr.killAll();
  console.log("\npty-speed-test passed!");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
