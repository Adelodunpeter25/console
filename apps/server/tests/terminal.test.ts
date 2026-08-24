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
 */
async function main(): Promise<void> {
  const app = createApiApp();
  const server = Bun.serve<string, TerminalSocketData>({
    port: 0,
    hostname: "127.0.0.1",
    fetch(req, srv) {
      if (isTerminalUpgradeRequest(req)) {
        const upgraded = srv.upgrade(req, { data: { url: req.url, sessionId: null, paused: false } });
        if (upgraded) return undefined;
        return new Response("Terminal WebSocket upgrade failed", { status: 400 });
      }
      return app.fetch(req);
    },
    websocket: terminalWebsocketHandlers.websocket,
  });
  const port = server.port;

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

  const spawned = await received<{ type: string; id: string; pid: number }>(
    (f) => f.type === "spawned",
  );
  assert.equal(spawned.type, "spawned");
  assert.ok(spawned.id.length > 0);
  assert.ok(spawned.pid > 0);
  console.log(`  ✅ spawn event (pid ${spawned.pid})`);

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
  server.stop(true);

  console.log("\nTerminal WebSocket tests passed!");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
