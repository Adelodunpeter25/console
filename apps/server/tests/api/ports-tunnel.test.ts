import assert from "node:assert/strict";
import {
  isPortTunnelUpgradeRequest,
  parseTunnelPort,
  portTunnelWebsocketHandlers,
  type PortTunnelSocketData,
} from "@/api/src/services/port-tunnel.socket.js";

console.log("Running port tunnel tests...");

assert.equal(parseTunnelPort("http://127.0.0.1:3000/api/ports/8000/tunnel"), 8000);
assert.equal(parseTunnelPort("http://127.0.0.1:3000/api/ports/8000/tunnel/"), 8000);
assert.equal(parseTunnelPort("http://127.0.0.1:3000/api/ports/abc/tunnel"), null);
assert.equal(parseTunnelPort("http://127.0.0.1:3000/api/ports/22/tunnel"), null);
assert.equal(
  isPortTunnelUpgradeRequest(
    new Request("http://127.0.0.1:3000/api/ports/8000/tunnel", {
      headers: { upgrade: "websocket" },
    }),
  ),
  true,
);
assert.equal(
  isPortTunnelUpgradeRequest(new Request("http://127.0.0.1:3000/api/ports/8000/tunnel")),
  false,
);
console.log("  ✅ tunnel URL parsing");

// Fake dev server on 127.0.0.1 (ephemeral port) with React-like routes.
const target = Bun.serve({
  port: 0,
  hostname: "127.0.0.1",
  fetch(request) {
    const url = new URL(request.url);
    if (url.pathname === "/") return new Response("<div id=root>app</div>");
    return new Response(`target:${url.pathname}`);
  },
});
const targetPort = target.port;
assert.ok(typeof targetPort === "number");
console.log(`  ✅ fake dev server on 127.0.0.1:${targetPort}`);

// Gateway wired exactly like apps/server/index.ts (tunnel branch only).
const gateway = Bun.serve<PortTunnelSocketData>({
  port: 0,
  hostname: "127.0.0.1",
  idleTimeout: 0,
  fetch(req, server) {
    if (isPortTunnelUpgradeRequest(req)) {
      const port = parseTunnelPort(req.url);
      if (port === null) return new Response("Invalid tunnel port", { status: 400 });
      const upgraded = server.upgrade(req, {
        data: { kind: "tunnel", port, url: req.url },
      });
      if (upgraded) return undefined;
      return new Response("Port tunnel upgrade failed", { status: 400 });
    }
    return new Response("Not found", { status: 404 });
  },
  websocket: {
    data: {} as PortTunnelSocketData,
    open: (ws) => portTunnelWebsocketHandlers.open(ws),
    message: (ws, data) => portTunnelWebsocketHandlers.message(ws, data),
    close: (ws) => portTunnelWebsocketHandlers.close(ws),
  },
});
const gatewayPort = gateway.port;
assert.ok(typeof gatewayPort === "number");

async function tunnelHttp(path: string): Promise<string> {
  const ws = new WebSocket(`ws://127.0.0.1:${gatewayPort}/api/ports/${targetPort}/tunnel`);
  ws.binaryType = "arraybuffer";
  const chunks: Uint8Array[] = [];
  const closed = new Promise<void>((resolve) => {
    ws.addEventListener("close", () => resolve());
    // Failsafe so a hung upstream can't stall the suite.
    setTimeout(() => resolve(), 8000);
  });
  ws.addEventListener("message", (event) => {
    if (typeof event.data === "string") {
      chunks.push(new TextEncoder().encode(event.data));
    } else {
      chunks.push(new Uint8Array(event.data as ArrayBuffer));
    }
  });
  await new Promise<void>((resolve, reject) => {
    ws.addEventListener("open", () => resolve());
    ws.addEventListener("error", () => reject(new Error("tunnel WS failed to open")));
  });
  ws.send(`GET ${path} HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n`);
  await closed;
  try {
    ws.close();
  } catch {
    // Already closed by the upstream.
  }
  const total = chunks.reduce((n, c) => n + c.length, 0);
  const merged = new Uint8Array(total);
  let offset = 0;
  for (const c of chunks) {
    merged.set(c, offset);
    offset += c.length;
  }
  return new TextDecoder().decode(merged);
}

try {
  const root = await tunnelHttp("/");
  assert.ok(root.includes("200"), "expected HTTP 200 through tunnel");
  assert.ok(root.includes('<div id=root>app</div>'), "expected dev-server body through tunnel");
  console.log("  ✅ tunnel proxies / (React root)");

  const asset = await tunnelHttp("/assets/index-abc123.js");
  assert.ok(asset.includes("target:/assets/index-abc123.js"), "expected path-preserving proxy");
  console.log("  ✅ tunnel preserves asset paths (relative dist loading)");
} finally {
  gateway.stop(true);
  target.stop(true);
}

console.log("Port tunnel tests passed!\n");
