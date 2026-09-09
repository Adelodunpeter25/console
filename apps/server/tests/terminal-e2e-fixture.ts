/**
 * Minimal terminal WS fixture for desktop end-to-end tests: boots the real
 * PTY manager + WebSocket handlers on a fixed port, no auth, nothing else.
 * Usage: bun tests/terminal-e2e-fixture.ts <port>
 */
import { isTerminalUpgradeRequest, terminalWebsocketHandlers } from "../api/src/terminal/socket.route.js";
import type { TerminalSocketData } from "../api/src/terminal/socket.route.js";

const port = Number(process.argv[2] ?? 0);

const server = Bun.serve<TerminalSocketData>({
  port,
  hostname: "127.0.0.1",
  fetch(req, srv) {
    if (isTerminalUpgradeRequest(req)) {
      const upgraded = srv.upgrade(req, {
        data: { url: req.url, sessionId: null, paused: false, binary: false },
      });
      if (upgraded) return undefined;
      return new Response("Terminal WebSocket upgrade failed", { status: 400 });
    }
    return new Response("ok");
  },
  websocket: terminalWebsocketHandlers.websocket,
});

console.log(`READY ${server.port}`);
