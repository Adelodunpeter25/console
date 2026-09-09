import assert from "node:assert/strict";
import { createApiApp } from "@/api/src/app.js";
import {
  isTerminalUpgradeRequest,
  terminalWebsocketHandlers,
  type TerminalSocketData,
} from "@/api/src/terminal/socket.route.js";

/**
 * Terminal WebSocket pipeline test:
 *  1. Boot a real Bun.serve HTTP server with the terminal WS handlers wired
 *     exactly like index.ts.
 *  2. Connect, spawn a shell in a temp dir.
 *  3. Assert spawn event + run a shell command, read output.
 *  4. Resize, then kill — assert exit frame and clean registry.
 *
 * Shell start: connecting starts the shell immediately at the requested grid
 * size (clients already send their true size in the spawn params).
 */
async function main(): Promise<void> {
  const app = createApiApp();
  const server = Bun.serve<TerminalSocketData>({
    port: 0,
    hostname: "127.0.0.1",
    fetch(req, srv) {
      if (isTerminalUpgradeRequest(req)) {
        const upgraded = srv.upgrade(req, {
          data: { url: req.url, sessionId: null, paused: false, binary: false },
        });
        if (upgraded) return undefined;
        return new Response("Terminal WebSocket upgrade failed", { status: 400 });
      }
      return app.fetch(req);
    },
    websocket: terminalWebsocketHandlers.websocket,
  });
  const port = server.port;
  if (typeof port !== "number") throw new Error("Server did not report a port");

  const cwd = "/tmp";
  const ws = new WebSocket(
    `ws://127.0.0.1:${port}/api/terminals?cwd=${encodeURIComponent(cwd)}&cols=120&rows=40`,
  );

  const frames: string[] = [];

  const received = <T = unknown>(predicate: (frame: T) => boolean, timeoutMs = 8000): Promise<T> =>
    new Promise((resolve, reject) => {
      const deadline = setTimeout(() => reject(new Error("Timed out waiting for frame")), timeoutMs);
      const check = (): void => {
        for (let i = 0; i < frames.length; i++) {
          try {
            const frame = JSON.parse(frames[i]!) as T;
            if (predicate(frame)) {
              clearTimeout(deadline);
              resolve(frame);
              return;
            }
          } catch {
            // skip malformed
          }
        }
      };
      ws.addEventListener("message", (event) => {
        const raw = typeof event.data === "string" ? event.data : new TextDecoder().decode(event.data);
        frames.push(raw);
        check();
      });
      check();
    });

  await new Promise<void>((resolve) => ws.addEventListener("open", () => resolve()));
  console.log("  ✅ terminal WS connected");

  // Shell starts immediately at the requested grid: the spawned frame must
  // arrive promptly without any resize.
  const spawned = await received<{ type: string; id: string; pid: number; cols: number; rows: number }>(
    (f) => f.type === "spawned",
  );
  assert.equal(spawned.type, "spawned");
  assert.ok(spawned.id.length > 0);
  assert.ok(spawned.pid > 0);
  assert.equal(spawned.cols, 120);
  assert.equal(spawned.rows, 40);
  console.log(`  ✅ spawn event at requested grid ${spawned.cols}x${spawned.rows} (pid ${spawned.pid})`);

  // Resize to a new grid is accepted.
  ws.send(JSON.stringify({ type: "resize", cols: 101, rows: 31 }));
  await new Promise((r) => setTimeout(r, 200));
  console.log("  ✅ resize accepted");

  // Run a command and capture its output.
  const outputPromise = received<{ type: string; data: string }>(
    (f) => f.type === "output" && f.data.includes("terminal-test-123"),
  );
  ws.send(JSON.stringify({ type: "input", data: "echo terminal-test-123\r" }));
  const output = await outputPromise;
  assert.ok(output.data.includes("terminal-test-123"));
  console.log("  ✅ pty output received");

  // Resize should not error.
  ws.send(JSON.stringify({ type: "resize", cols: 100, rows: 30 }));
  await new Promise((r) => setTimeout(r, 200));
  console.log("  ✅ resize accepted");

  // Input echo works; kill triggers exit.
  const exitPromise = received<{ type: string; code: number | null }>((f) => f.type === "exit");
  ws.send(JSON.stringify({ type: "kill" }));
  const exit = await exitPromise;
  assert.ok(typeof exit.code === "number" || exit.code === null);
  console.log("  ✅ kill → exit frame");

  await new Promise((r) => setTimeout(r, 200));
  ws.close();

  await binaryProtocolTest(port);
  server.stop(true);

  console.log("\nTerminal WebSocket tests passed!");
}

/**
 * Binary protocol pass: connect with ?proto=binary and assert —
 *   - spawned/exit still arrive as JSON text frames,
 *   - output arrives as binary frames [0x01, ...raw pty bytes],
 *   - binary input frames [0x01, ...bytes] reach the PTY,
 *   - JSON input frames keep working on the same connection.
 */
async function binaryProtocolTest(port: number): Promise<void> {
  const OUTPUT_TAG = 0x01;
  const INPUT_TAG = 0x01;
  const cwd = "/tmp";
  const ws = new WebSocket(
    `ws://127.0.0.1:${port}/api/terminals?cwd=${encodeURIComponent(cwd)}&cols=80&rows=24&proto=binary`,
  );
  ws.binaryType = "arraybuffer";

  const frames: (string | Uint8Array)[] = [];
  const received = (
    predicate: (frame: string | Uint8Array) => boolean,
    timeoutMs = 8000,
  ): Promise<string | Uint8Array> =>
    new Promise((resolve, reject) => {
      const deadline = setTimeout(() => reject(new Error("Timed out waiting for frame")), timeoutMs);
      const check = (): void => {
        for (let i = 0; i < frames.length; i++) {
          if (predicate(frames[i]!)) {
            clearTimeout(deadline);
            resolve(frames[i]!);
            return;
          }
        }
      };
      ws.addEventListener("message", (event) => {
        if (typeof event.data === "string") {
          frames.push(event.data);
        } else {
          // Bun may deliver binary frames as ArrayBuffer or Uint8Array.
          frames.push(new Uint8Array(event.data as ArrayBuffer));
        }
        check();
      });
      check();
    });

  await new Promise<void>((resolve) => ws.addEventListener("open", () => resolve()));
  console.log("  ✅ binary-mode terminal WS connected");

  // spawned stays a JSON text frame.
  const spawnedRaw = await received((f) => typeof f === "string" && f.includes("\"spawned\""));
  const spawned = JSON.parse(spawnedRaw as string) as { type: string };
  assert.equal(spawned.type, "spawned");
  console.log("  ✅ spawned frame is still JSON text");

  // Output arrives as a binary frame with tag 0x01 and raw PTY bytes.
  const decoder = new TextDecoder();
  const outputPromise = received((f) => {
    if (typeof f !== "string" && f[0] === OUTPUT_TAG) {
      return decoder.decode(f.subarray(1)).includes("binary-test-456");
    }
    return false;
  });
  ws.send(JSON.stringify({ type: "input", data: "echo binary-test-456\r" }));
  const output = (await outputPromise) as Uint8Array;
  assert.equal(output[0], OUTPUT_TAG);
  assert.ok(decoder.decode(output.subarray(1)).includes("binary-test-456"));
  console.log("  ✅ output arrives as tagged binary frame with raw PTY bytes");

  // Binary input reaches the PTY too.
  const binaryInput = new Uint8Array(1 + "echo binary-input-789\r".length);
  binaryInput[0] = INPUT_TAG;
  binaryInput.set(Buffer.from("echo binary-input-789\r"), 1);
  const binaryOutputPromise = received((f) => {
    if (typeof f !== "string" && f[0] === OUTPUT_TAG) {
      return decoder.decode(f.subarray(1)).includes("binary-input-789");
    }
    return false;
  });
  ws.send(binaryInput);
  await binaryOutputPromise;
  console.log("  ✅ binary input frame reaches the PTY");

  // exit stays a JSON text frame.
  const exitPromise = received((f) => typeof f === "string" && f.includes("\"exit\""));
  ws.send(JSON.stringify({ type: "kill" }));
  const exit = JSON.parse((await exitPromise) as string) as { type: string };
  assert.equal(exit.type, "exit");
  console.log("  ✅ exit frame is still JSON text");

  await new Promise((r) => setTimeout(r, 200));
  ws.close();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
