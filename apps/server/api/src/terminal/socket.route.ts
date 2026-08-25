/**
 * Terminal WebSocket endpoint (/api/terminals).
 *
 * Uses Bun.serve's native WebSockets (replacing the `ws` shim): HTTP requests
 * flow through `fetch`, while `/api/terminals` upgrade requests are handed to
 * `server.upgrade` and thereafter driven by the exported handlers.
 *
 * Protocol:
 *   - Connect:  GET /api/terminals?cwd=...&cols=...&rows=...&shell=...&label=...
 *   - Server→Client frames: JSON `TerminalServerMessage`
 *     { type: "spawned" | "output" | "exit" | "error", ... }
 *   - Client→Server frames: JSON `TerminalClientMessage`
 *     { type: "input" | "resize" | "kill", ... }
 */
import type {
  TerminalClientMessage,
  TerminalSpawnParams,
} from "@console/types";
import { terminalPtyManager } from "./pty.manager.js";

const TERMINAL_PATH = "/api/terminals";

/** Per-socket state attached at upgrade time and mutated over the lifetime. */
export interface TerminalSocketData {
  /** The original request URL carrying spawn params. */
  url: string;
  /** PTY session spawned for this socket; null until `open` succeeds. */
  sessionId: string | null;
  /** True while output is paused due to send-buffer backpressure. */
  paused: boolean;
  /** Interval polling the buffered amount while paused (resume trigger). */
  drainPoller?: ReturnType<typeof setInterval>;
}

export function isTerminalUpgradeRequest(req: Request): boolean {
  if (req.headers.get("upgrade") !== "websocket") return false;
  try {
    return new URL(req.url).pathname === TERMINAL_PATH;
  } catch {
    return false;
  }
}

// Pause the PTY when the socket send buffer saturates so flooding programs
// can't grow memory without bound; resume once it drains below the low mark.
const HIGH_WATER = 1 << 20; // 1 MiB
const LOW_WATER = HIGH_WATER / 2;

export const terminalWebsocketHandlers = {
  websocket: {
    data: {} as TerminalSocketData,
    open(ws: import("bun").ServerWebSocket<TerminalSocketData>): void {
      ws.data.sessionId = null;
      ws.data.paused = false;

      const maybeResume = () => {
        if (!ws.data.sessionId || !ws.data.paused) return;
        if (ws.getBufferedAmount() <= LOW_WATER) {
          ws.data.paused = false;
          terminalPtyManager.resume(ws.data.sessionId);
          if (ws.data.drainPoller) {
            clearInterval(ws.data.drainPoller);
            ws.data.drainPoller = undefined;
          }
        }
      };
      // Native WebSockets have no "drain" event; poll the buffered amount
      // while paused and resume once it falls below the low-water mark.
      const checkBackpressure = () => {
        if (!ws.data.sessionId || ws.data.paused) return;
        if (ws.getBufferedAmount() > HIGH_WATER) {
          ws.data.paused = true;
          terminalPtyManager.pause(ws.data.sessionId);
          if (!ws.data.drainPoller) ws.data.drainPoller = setInterval(maybeResume, 25);
        }
      };
      const send = (message: unknown) => {
        ws.send(JSON.stringify(message));
        checkBackpressure();
      };

      // Validate cwd before spawning so the client gets a clean error frame.
      try {
        const params = parseSpawnParams(new URL(ws.data.url));
        const spawned = terminalPtyManager.spawn(params);
        ws.data.sessionId = spawned.id;
        terminalPtyManager.attach(spawned.id, {
          onData: (event) => send(event),
          onExit: (code) => send({ type: "exit", code }),
          onError: (message) => send({ type: "error", message }),
        });
        send(spawned);
      } catch (err) {
        send({ type: "error", message: err instanceof Error ? err.message : String(err) });
        ws.close(4000, "Spawn failed");
      }
    },

    message(
      ws: import("bun").ServerWebSocket<TerminalSocketData>,
      data: string | Uint8Array,
    ): void {
      const session = ws.data.sessionId;
      if (!session) return;

      let frame: TerminalClientMessage;
      try {
        frame = JSON.parse(
          typeof data === "string" ? data : new TextDecoder().decode(data),
        ) as TerminalClientMessage;
      } catch {
        ws.send(JSON.stringify({ type: "error", message: "Invalid terminal frame: expected JSON." }));
        return;
      }

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
          ws.send(JSON.stringify({ type: "error", message: `Unknown terminal frame type: ${(frame as { type?: string }).type}` }));
      }
    },

    close(ws: import("bun").ServerWebSocket<TerminalSocketData>): void {
      if (ws.data.drainPoller) clearInterval(ws.data.drainPoller);
      const session = ws.data.sessionId;
      if (session) {
        terminalPtyManager.kill(session);
        ws.data.sessionId = null;
      }
    },
  },
};

function parseSpawnParams(url: URL): TerminalSpawnParams {
  const num = (v: string | null, fallback: number) => {
    if (!v) return fallback;
    const n = Number.parseInt(v, 10);
    return Number.isFinite(n) && n > 0 ? n : fallback;
  };
  return {
    cwd: url.searchParams.get("cwd") ?? process.cwd(),
    shell: url.searchParams.get("shell") ?? undefined,
    cols: num(url.searchParams.get("cols"), 80),
    rows: num(url.searchParams.get("rows"), 24),
    label: url.searchParams.get("label") ?? undefined,
  };
}
