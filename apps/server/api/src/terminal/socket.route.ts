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

/**
 * Binary frame tags for `proto=binary` connections (see docs/plan/terminal-binary-protocol-plan.md).
 * Binary frames are `[tag, ...payload]`; control messages stay JSON text frames.
 */
export const TERMINAL_OUTPUT_FRAME_TAG = 0x01;
export const TERMINAL_INPUT_FRAME_TAG = 0x01;

/** Per-socket state attached at upgrade time and mutated over the lifetime. */
export interface TerminalSocketData {
  /** The original request URL carrying spawn params. */
  url: string;
  /** PTY session spawned for this socket; null until `open` succeeds. */
  sessionId: string | null;
  /** True when the client negotiated the binary protocol via ?proto=binary. */
  binary: boolean;
  /** Stream decoder used only by JSON-compat clients (output bytes → text). */
  compatDecoder?: TextDecoder;
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

      // Binary protocol opt-in: ?proto=binary on the upgrade URL. Clients that
      // omit it get the legacy JSON-only behavior (output re-decoded here).
      ws.data.binary =
        parseSpawnParams(new URL(ws.data.url)).proto === "binary";

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
      const sendJson = (message: unknown) => {
        ws.send(JSON.stringify(message));
        checkBackpressure();
      };
      const sendOutput = (chunk: Uint8Array) => {
        if (ws.data.binary) {
          // Bun sends a binary WS frame when given non-string data.
          const frame = new Uint8Array(chunk.byteLength + 1);
          frame[0] = TERMINAL_OUTPUT_FRAME_TAG;
          frame.set(chunk, 1);
          ws.send(frame);
        } else {
          // JSON compat: decode PTY bytes to text (stream-aware so multibyte
          // sequences split across chunks survive) and ship the old shape.
          const decoder = (ws.data.compatDecoder ??= new TextDecoder("utf-8", {
            fatal: false,
          }));
          const text = decoder.decode(chunk, { stream: true });
          if (text.length > 0) sendJson({ type: "output", data: text });
          return;
        }
        checkBackpressure();
      };

      // Validate cwd before spawning so the client gets a clean error frame.
      try {
        const params = parseSpawnParams(new URL(ws.data.url));
        // spawn() creates the PTY and starts the shell synchronously (id is
        // usable right away). Do NOT await `ready` here: Bun holds WS messages
        // until an async open resolves, which would deadlock client input.
        const { id, ready } = terminalPtyManager.spawn(params);
        ws.data.sessionId = id;
        terminalPtyManager.attach(id, {
          onData: (chunk) => sendOutput(chunk),
          onExit: (code) => sendJson({ type: "exit", code }),
          onError: (message) => sendJson({ type: "error", message }),
        });
        void ready.then(
          (spawned) => sendJson(spawned),
          (err: unknown) => {
            // The close handler kills pre-start sessions, so by the time this
            // rejection lands the socket may already be gone — never rethrow.
            try {
              sendJson({ type: "error", message: err instanceof Error ? err.message : String(err) });
              ws.close(4000, "Spawn failed");
            } catch {
              // Socket already closed — nothing to report.
            }
          },
        );
      } catch (err) {
        sendJson({ type: "error", message: err instanceof Error ? err.message : String(err) });
        ws.close(4000, "Spawn failed");
      }
    },

    message(
      ws: import("bun").ServerWebSocket<TerminalSocketData>,
      data: string | Uint8Array,
    ): void {
      // Reject oversized frames (cheap DoS/CPU guard)
      const size = typeof data === "string" ? Buffer.byteLength(data) : (data as Uint8Array).byteLength;
      if (size > 1024 * 1024) {
        ws.send(JSON.stringify({ type: "error", message: "Frame too large." }));
        return;
      }
      const session = ws.data.sessionId;
      if (!session) return;

      // Binary frames: [tag, ...payload]. Only input exists client→server.
      if (typeof data !== "string") {
        if (!ws.data.binary) {
          ws.send(JSON.stringify({ type: "error", message: "Binary frames require ?proto=binary." }));
          return;
        }
        if (data.byteLength === 0) return;
        if (data[0] === TERMINAL_INPUT_FRAME_TAG) {
          terminalPtyManager.write(session, data.subarray(1));
          return;
        }
        ws.send(JSON.stringify({ type: "error", message: `Unknown binary frame tag: ${data[0]}` }));
        return;
      }

      let frame: TerminalClientMessage;
      try {
        frame = JSON.parse(data) as TerminalClientMessage;
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
  const num = (v: string | null, fallback: number, max: number) => {
    if (!v) return fallback;
    const n = Number.parseInt(v, 10);
    if (!Number.isFinite(n) || n <= 0) return fallback;
    return Math.min(n, max);
  };
  const rawCwd = url.searchParams.get("cwd") ?? process.cwd();
  if (rawCwd.length > 4096) throw new Error("cwd too long");
  // Prevent empty or null-byte cwd
  if (!rawCwd || rawCwd.includes("\0")) throw new Error("Invalid cwd");
  const rawShell = url.searchParams.get("shell") ?? undefined;
  if (rawShell && rawShell.length > 1024) throw new Error("shell path too long");
  if (rawShell && rawShell.includes("\0")) throw new Error("Invalid shell");
  const rawLabel = url.searchParams.get("label") ?? undefined;
  if (rawLabel && rawLabel.length > 256) throw new Error("label too long");
  // Binary protocol opt-in; anything else is rejected so typos fail loudly.
  const rawProto = url.searchParams.get("proto");
  if (rawProto && rawProto !== "binary") throw new Error(`Unsupported proto: ${rawProto}`);
  return {
    cwd: rawCwd,
    shell: rawShell,
    cols: num(url.searchParams.get("cols"), 80, 500),
    rows: num(url.searchParams.get("rows"), 24, 200),
    label: rawLabel,
    proto: rawProto === "binary" ? "binary" : undefined,
  };
}
