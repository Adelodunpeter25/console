/**
 * Port tunnel WebSocket endpoint (/api/ports/:port/tunnel).
 *
 * Lets a remote desktop app expose a VPS-local dev server as a real
 * localhost port on the desktop:
 *
 *   desktop TcpListener 127.0.0.1:48000 <--ws--> this endpoint <--tcp--> 127.0.0.1:8000
 *
 * Protocol is raw TCP over WebSocket binary frames: every WS message is one
 * TCP chunk in either direction. No HTTP rewriting happens here, so React
 * dist assets (/assets/..., /vite.svg), absolute paths, and HMR websockets
 * behave exactly like a local dev server.
 *
 * Auth: same trust model as the rest of the API (currently open LAN/VPS).
 * No extra token — reuse whatever the main connection uses.
 */
import { Socket } from "node:net";

export interface PortTunnelSocketData {
  kind: "tunnel";
  /** Remote dev-server port on this host (127.0.0.1). */
  port: number;
  /** Original upgrade URL (debug only). */
  url: string;
  socket?: Socket;
  closed?: boolean;
  drainPoller?: ReturnType<typeof setInterval>;
}

const TUNNEL_PATH = /^\/api\/ports\/(\d+)\/tunnel\/?$/;

export function parseTunnelPort(url: string): number | null {
  try {
    const pathname = new URL(url).pathname;
    const match = TUNNEL_PATH.exec(pathname);
    if (!match) return null;
    const port = Number.parseInt(match[1]!, 10);
    if (!Number.isInteger(port) || port < 1_024 || port > 65_535) return null;
    return port;
  } catch {
    return null;
  }
}

export function isPortTunnelUpgradeRequest(req: Request): boolean {
  if (req.headers.get("upgrade") !== "websocket") return false;
  try {
    return TUNNEL_PATH.test(new URL(req.url).pathname);
  } catch {
    return false;
  }
}

const HIGH_WATER = 1 << 20;
const LOW_WATER = HIGH_WATER / 2;

export const portTunnelWebsocketHandlers = {
  open(ws: import("bun").ServerWebSocket<PortTunnelSocketData>): void {
    const port = ws.data.port;
    if (!Number.isInteger(port) || port < 1_024 || port > 65_535) {
      ws.close(4400, "Invalid port");
      return;
    }

    const socket = new Socket();
    ws.data.socket = socket;
    ws.data.closed = false;

    const maybeResume = () => {
      if (ws.data.closed) return;
      try {
        if (ws.getBufferedAmount() <= LOW_WATER) {
          socket.resume();
          if (ws.data.drainPoller) {
            clearInterval(ws.data.drainPoller);
            ws.data.drainPoller = undefined;
          }
        }
      } catch {
        // Socket already gone.
      }
    };

    socket.on("data", (chunk: Buffer) => {
      if (ws.data.closed) return;
      try {
        ws.send(chunk);
      } catch {
        socket.destroy();
        return;
      }
      try {
        if (ws.getBufferedAmount() > HIGH_WATER) {
          socket.pause();
          if (!ws.data.drainPoller) {
            ws.data.drainPoller = setInterval(maybeResume, 25);
          }
        }
      } catch {
        // getBufferedAmount can throw after close — ignore.
      }
    });
    socket.on("error", () => {
      try {
        ws.close(1011, "Upstream connection failed");
      } catch {
        // Already closed.
      }
    });
    socket.on("close", () => {
      if (ws.data.drainPoller) {
        clearInterval(ws.data.drainPoller);
        ws.data.drainPoller = undefined;
      }
      if (!ws.data.closed) {
        try {
          ws.close(1000, "Upstream closed");
        } catch {
          // Already closed.
        }
      }
    });

    socket.connect(port, "127.0.0.1");
  },

  message(
    ws: import("bun").ServerWebSocket<PortTunnelSocketData>,
    data: string | Uint8Array,
  ): void {
    const socket = ws.data.socket;
    if (!socket || socket.destroyed || ws.data.closed) return;
    try {
      if (typeof data === "string") {
        socket.write(data);
      } else {
        socket.write(data);
      }
    } catch {
      // Write after destroy — the close handler cleans up.
    }
  },

  close(ws: import("bun").ServerWebSocket<PortTunnelSocketData>): void {
    ws.data.closed = true;
    if (ws.data.drainPoller) {
      clearInterval(ws.data.drainPoller);
      ws.data.drainPoller = undefined;
    }
    try {
      ws.data.socket?.destroy();
    } catch {
      // Ignore destroy errors.
    }
    ws.data.socket = undefined;
  },
};
