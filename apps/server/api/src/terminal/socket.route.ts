/**
 * Terminal WebSocket endpoint (/api/terminals).
 *
 * Attaches a `ws` WebSocketServer to the raw Node http.Server created in
 * index.ts. The Hono app cannot upgrade sockets itself (the server uses
 * `createServer` + `app.fetch`), so we intercept the `upgrade` event for
 * `/api/terminals` and hand the socket to node-pty.
 *
 * Protocol:
 *   - Connect:  GET /api/terminals?cwd=...&cols=...&rows=...&shell=...&label=...
 *   - Server→Client frames: JSON `TerminalServerMessage`
 *     { type: "spawned" | "output" | "exit" | "error", ... }
 *   - Client→Server frames: JSON `TerminalClientMessage`
 *     { type: "input" | "resize" | "kill", ... }
 */
import type { Server } from "node:http";
import { WebSocketServer, type WebSocket } from "ws";
import type {
  TerminalClientMessage,
  TerminalSpawnParams,
} from "@console/types";
import { terminalPtyManager } from "./pty.manager.js";

const TERMINAL_PATH = "/api/terminals";

function isTerminalRequest(url: string | undefined): boolean {
  if (!url) return false;
  try {
    const pathname = new URL(url, "http://localhost").pathname;
    return pathname === TERMINAL_PATH;
  } catch {
    return false;
  }
}

/**
 * Mount the terminal WebSocket server onto the HTTP server.
 * Call once from the server entry point; returns a close function for shutdown.
 */
export function attachTerminalSocket(server: Server): () => void {
  const wss = new WebSocketServer({ noServer: true });

  server.on("upgrade", (req, socket, head) => {
    if (!isTerminalRequest(req.url)) return; // other consumers (none yet) or plain HTTP
    wss.handleUpgrade(req, socket, head, (ws) => {
      wss.emit("connection", ws, req);
    });
  });

  wss.on("connection", (ws: WebSocket, req) => {
    const params = parseSpawnParams(req.url);

    let session: TerminalId | null = null;
    const send = (message: unknown) => {
      if (ws.readyState === ws.OPEN) ws.send(JSON.stringify(message));
    };

    // Validate cwd before spawning so the client gets a clean error frame.
    try {
      const spawned = terminalPtyManager.spawn(params);
      session = spawned.id;
      terminalPtyManager.attach(spawned.id, {
        onData: (event) => send(event),
        onExit: (code) => send({ type: "exit", code }),
        onError: (message) => send({ type: "error", message }),
      });
      send(spawned);
    } catch (err) {
      send({ type: "error", message: err instanceof Error ? err.message : String(err) });
      ws.close(4000, "Spawn failed");
      return;
    }

    ws.on("message", (raw) => {
      let frame: TerminalClientMessage;
      try {
        frame = JSON.parse(raw.toString("utf-8")) as TerminalClientMessage;
      } catch {
        send({ type: "error", message: "Invalid terminal frame: expected JSON." });
        return;
      }

      if (!session) return;

      switch (frame.type) {
        case "input":
          terminalPtyManager.write(session, frame.data);
          break;
        case "resize":
          terminalPtyManager.resize(session, frame.cols, frame.rows);
          break;
        case "kill":
          terminalPtyManager.kill(session);
          ws.close(1000, "Killed");
          break;
        default:
          send({ type: "error", message: `Unknown terminal frame type: ${(frame as { type?: string }).type}` });
      }
    });

    ws.on("close", () => {
      if (session) {
        terminalPtyManager.kill(session);
        session = null;
      }
    });

    ws.on("error", () => {
      // Socket error — the close handler still fires; nothing extra needed.
    });
  });

  return () => wss.close();
}

function parseSpawnParams(url: string | undefined): TerminalSpawnParams {
  const parsed = new URL(url ?? "/", "http://localhost");
  const num = (v: string | null, fallback: number) => {
    if (!v) return fallback;
    const n = Number.parseInt(v, 10);
    return Number.isFinite(n) && n > 0 ? n : fallback;
  };
  return {
    cwd: parsed.searchParams.get("cwd") ?? process.cwd(),
    shell: parsed.searchParams.get("shell") ?? undefined,
    cols: num(parsed.searchParams.get("cols"), 80),
    rows: num(parsed.searchParams.get("rows"), 24),
    label: parsed.searchParams.get("label") ?? undefined,
  };
}