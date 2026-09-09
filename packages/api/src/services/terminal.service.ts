import type {
  TerminalClientMessage,
  TerminalServerMessage,
  TerminalSpawnParams,
  TerminalSpawnedEvent,
} from "@console/types";

/**
 * Interactive terminal WebSocket client for mobile/web clients that talk to
 * the Console server directly (the desktop app uses the Rust relay commands
 * in `src-tauri` instead).
 *
 * Protocol:
 *   - Connect to `{baseUrl}/api/terminals?cwd=...&cols=...&rows=...&proto=binary`
 *   - Server → client: binary `output` frames `[0x01, ...raw pty bytes]`;
 *     JSON text frames for `spawned`/`exit`/`error`
 *   - Client → server: binary `input` frames `[0x01, ...keystroke bytes]`;
 *     JSON text frames for `resize`/`kill` (JSON `input` also accepted)
 *
 * Callers always see the JSON-shaped `TerminalServerMessage` union — binary
 * framing is decoded here and never leaks out of this module.
 */

/** Binary frame tags — must mirror the server's socket.route.ts constants. */
const OUTPUT_FRAME_TAG = 0x01;
const INPUT_FRAME_TAG = 0x01;

const hasTextDecoder = typeof TextDecoder !== "undefined";
const hasTextEncoder = typeof TextEncoder !== "undefined";

/**
 * Streaming UTF-8 decoder for binary frame payloads. Uses the platform
 * `TextDecoder` (stream mode) when available and falls back to a manual
 * decoder that carries incomplete multi-byte sequences across frames —
 * Hermes does not ship `TextDecoder`.
 */
class Utf8StreamDecoder {
  private platform: TextDecoder | null = hasTextDecoder ? new TextDecoder("utf-8") : null;
  private residue: number[] = [];

  decode(bytes: Uint8Array): string {
    if (this.platform) return this.platform.decode(bytes, { stream: true });

    let input = bytes;
    if (this.residue.length > 0) {
      const merged = new Uint8Array(this.residue.length + bytes.length);
      merged.set(this.residue, 0);
      merged.set(bytes, this.residue.length);
      input = merged;
      this.residue = [];
    }

    // Hold back a trailing incomplete multi-byte sequence until the next frame.
    let end = input.length;
    for (let i = input.length - 1; i >= 0 && i >= input.length - 3; i--) {
      const b = input[i]!;
      if ((b & 0xc0) === 0x80) continue; // continuation byte — scan back to the lead
      if ((b & 0x80) === 0) break; // ASCII — frame ends on a boundary
      const need = (b & 0xe0) === 0xc0 ? 2 : (b & 0xf0) === 0xe0 ? 3 : 4;
      if (input.length - i < need) {
        for (let j = i; j < input.length; j++) this.residue.push(input[j]!);
        end = i;
      }
      break;
    }

    let out = "";
    let i = 0;
    while (i < end) {
      const b = input[i]!;
      if (b < 0x80) {
        out += String.fromCharCode(b);
        i += 1;
        continue;
      }
      let cp: number;
      let len: number;
      if ((b & 0xe0) === 0xc0) {
        cp = b & 0x1f;
        len = 2;
      } else if ((b & 0xf0) === 0xe0) {
        cp = b & 0x0f;
        len = 3;
      } else if ((b & 0xf8) === 0xf0) {
        cp = b & 0x07;
        len = 4;
      } else {
        out += "\uFFFD";
        i += 1;
        continue;
      }
      if (i + len > end) {
        out += "\uFFFD";
        i += 1;
        continue;
      }
      let valid = true;
      for (let j = 1; j < len; j++) {
        const c = input[i + j]!;
        if ((c & 0xc0) !== 0x80) {
          valid = false;
          break;
        }
        cp = (cp << 6) | (c & 0x3f);
      }
      out += String.fromCodePoint(valid ? cp : 0xfffd);
      i += len;
    }
    return out;
  }
}

/** UTF-8 encode a JS string for binary input frames (Hermes lacks `TextEncoder`). */
function utf8Encode(str: string): Uint8Array {
  if (hasTextEncoder) return new TextEncoder().encode(str);
  const out: number[] = [];
  for (let i = 0; i < str.length; i++) {
    const cp = str.codePointAt(i)!;
    if (cp > 0xffff) i++; // consume the low surrogate of a pair
    if (cp < 0x80) {
      out.push(cp);
    } else if (cp < 0x800) {
      out.push(0xc0 | (cp >> 6), 0x80 | (cp & 0x3f));
    } else if (cp < 0x10000) {
      out.push(0xe0 | (cp >> 12), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
    } else {
      out.push(
        0xf0 | (cp >> 18),
        0x80 | ((cp >> 12) & 0x3f),
        0x80 | ((cp >> 6) & 0x3f),
        0x80 | (cp & 0x3f),
      );
    }
  }
  return new Uint8Array(out);
}

export function connectTerminal(
  options: {
    baseUrl: string;
    params: TerminalSpawnParams;
    onEvent: (message: TerminalServerMessage) => void;
    onClose?: () => void;
    onError?: (message: string) => void;
  },
): { open: () => Promise<TerminalSpawnedEvent>; input: (data: string) => void; resize: (cols: number, rows: number) => void; kill: () => void; close: () => void } {
  const wsBase = options.baseUrl.replace(/^http/, "ws");
  const cwd = encodeURIComponent(options.params.cwd);
  const cols = options.params.cols ?? 80;
  const rows = options.params.rows ?? 24;
  const shell = options.params.shell
    ? `&shell=${encodeURIComponent(options.params.shell)}`
    : "";
  const label = options.params.label
    ? `&label=${encodeURIComponent(options.params.label)}`
    : "";

  // Opt into binary data frames; control messages stay JSON. Older servers
  // ignore the param and keep speaking pure JSON — handled below.
  const url = `${wsBase}/api/terminals?cwd=${cwd}&cols=${cols}&rows=${rows}${shell}${label}&proto=binary`;
  let ws: WebSocket | null = null;
  let spawnedPromise: Promise<TerminalSpawnedEvent> | null = null;

  const send = (message: TerminalClientMessage) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(message));
    }
  };

  const open = (): Promise<TerminalSpawnedEvent> => {
    if (spawnedPromise) return spawnedPromise;

    spawnedPromise = new Promise<TerminalSpawnedEvent>((resolve, reject) => {
      const socket = new WebSocket(url);
      socket.binaryType = "arraybuffer";
      ws = socket;
      const utf8 = new Utf8StreamDecoder();
      let settled = false;

      socket.onopen = () => {
        // The server sends the "spawned" confirmation first.
      };

      socket.onmessage = (event) => {
        let message: TerminalServerMessage;
        try {
          if (typeof event.data === "string") {
            // JSON text frame (control messages, or a legacy server).
            message = JSON.parse(event.data) as TerminalServerMessage;
          } else {
            // Binary frame: [tag, ...payload].
            const bytes =
              event.data instanceof Uint8Array ? event.data : new Uint8Array(event.data);
            if (bytes.length === 0) return;
            if (bytes[0] !== OUTPUT_FRAME_TAG) {
              options.onError?.(`Unknown terminal binary frame tag: ${bytes[0]}`);
              return;
            }
            const data = utf8.decode(bytes.subarray(1));
            if (data.length === 0) return; // residue only — nothing to emit yet
            message = { type: "output", data };
          }
        } catch {
          options.onError?.("Invalid terminal frame from server.");
          return;
        }

        if (!settled && message.type === "spawned") {
          settled = true;
          resolve(message);
        } else if (!settled && message.type === "error") {
          settled = true;
          reject(new Error(message.message));
          socket.close();
          return;
        }
        options.onEvent(message);
      };

      socket.onerror = () => {
        if (!settled) {
          settled = true;
          reject(new Error("Terminal WebSocket connection failed."));
        }
        options.onError?.("Terminal WebSocket connection failed.");
      };

      socket.onclose = () => {
        options.onClose?.();
      };
    });

    return spawnedPromise;
  };

  return {
    open,
    input: (data) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return;
      // Binary protocol: input rides a binary frame [0x01, ...bytes].
      const payload = utf8Encode(data);
      const frame = new Uint8Array(payload.length + 1);
      frame[0] = INPUT_FRAME_TAG;
      frame.set(payload, 1);
      ws.send(frame.buffer);
    },
    resize: (cols, rows) => send({ type: "resize", cols, rows }),
    kill: () => send({ type: "kill" }),
    close: () => {
      if (ws) ws.close();
    },
  };
}