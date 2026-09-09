# Terminal Binary Protocol Plan

## Goal

Replace JSON text frames on the terminal WebSocket hot path with binary WS
frames so that PTY bytes travel with the minimum number of copies. Today every
output byte is copied ~4–5 times and UTF-8 encoded/decoded twice:

```
PTY (bytes) → TextDecoder → string chunks → join("") → JSON.stringify (escape)
           → ws.send (UTF-8 encode) → client JSON.parse → string
```

Binary framing collapses this to:

```
PTY (bytes) → ws.sendBinary (raw payload) → client decodes once for the grid
```

This removes `JSON.stringify`/`JSON.parse`, all string joins, and one full
UTF-8 encode/decode round trip per byte — the two costs that dominate exactly
when a lot of data is being transferred (`cat` of a large file, builds, `git
log -p`).

## Scope

- **In scope:** the terminal WS protocol (`/api/terminals`), server-side
  output/input framing, and the three clients that speak it: desktop (Rust),
  mobile (React Native), and the server test suite.
- **Out of scope:** PTY lifecycle, coalescing windows, backpressure
  pause/resume mechanics (all stay as they are), and the REST terminal routes.
- Control-plane messages (`spawned`, `exit`, `error`, `resize`, `kill`) stay
  as JSON text frames. They are tiny and infrequent; only `output` and `input`
  move to binary.

## Wire format

**Server → Client (binary WS frame):**

```
byte 0:     tag (u8)
  0x01      OUTPUT  — bytes 1.. are raw UTF-8 straight from the PTY
bytes 1..:  payload
```

One socket is bound to exactly one terminal session (already true), so no
session id is needed inside the frame.

**Client → Server (binary WS frame):**

```
byte 0:     tag (u8)
  0x01      INPUT   — bytes 1.. are raw keystroke/paste bytes
bytes 1..:  payload
```

**Server → Client (JSON text frames, unchanged):** `spawned`, `exit`,
`error`. **Client → Server (JSON text frames, unchanged):** `resize`, `kill`.

## Compatibility / negotiation

The upgrade URL gains an optional query param: `GET /api/terminals?...&proto=binary`.

- Clients that send `proto=binary` get binary output frames and may send
  binary input frames.
- Clients that omit it get today's JSON-only behavior — old clients keep
  working with zero changes.

The server accepts both frame kinds from a `proto=binary` client (text JSON
control frames + binary data frames). A client is never required to send
binary input; JSON `input` frames keep working for every client.

## Phase 1 — Server (`apps/server/api/src/terminal/`)

1. **`pty.manager.ts`**
   - Change `PtyCallbacks.onData` to deliver the raw PTY chunk without
     decoding: keep `TextDecoder` only when the session is in JSON-compat
     mode. Simplest shape: the manager always hands through
     `Uint8Array`; the *route* decodes for JSON-compat clients. Move the
     decoder out of the session into the route's compat path.
   - `outputQueue` becomes `Uint8Array[]` with a byte counter; the flush join
     becomes a `Buffer.concat`-style copy (or send chunks individually — see
     the chunked-flush item below).
   - **Chunked flush (small fix riding along):** `flushOutput` must cap
     frames at `OUTPUT_FRAME_BYTES` the same way `resume()` already does, so
     a burst window cannot become one multi-megabyte frame.
   - `write()` accepts `string | Uint8Array` so binary input passes through
     without a string round trip.
2. **`socket.route.ts`**
   - Parse `proto` from the upgrade URL into `TerminalSocketData`.
   - `proto=binary`: `onData` sends `ws.sendBinary(tagged frame)`; the
     `message` handler gets a `Binary` arm that strips the tag and forwards
     input bytes. JSON text frames from the same client still work.
   - JSON compat: existing behavior, byte-for-byte.
3. **`packages/types` (`src/terminal.ts`)**
   - Add `"proto"?: "binary"` to the spawn params type and document the frame
     format above in a comment block. The TS message types for
     spawned/exit/error/resize/kill are unchanged.
4. **Tests (`apps/server/tests/terminal.test.ts`)**
   - Connect with `proto=binary`: assert output arrives as binary frames with
     tag `0x01` and payload equal to the PTY bytes; assert binary input
     reaches the PTY; assert `spawned`/`exit` still arrive as JSON text.
   - Connect without the param: assert the old JSON behavior is unchanged
     (existing tests already cover this — they must pass untouched).

## Phase 2 — Desktop (`apps/desktop/crates/console-core/src/services/terminal/`)

1. **`mod.rs`** (tokio-tungstenite client)
   - Append `&proto=binary` to the upgrade URL.
   - In the WS receive loop, add a `Message::Binary(bytes)` arm: check tag
     `0x01`, forward `&bytes[1..]` to the terminal backend (termy) as raw
     bytes. termy consumes bytes natively, so the client-side
     string-decode-then-reencode disappears too.
   - Keep the existing `Message::Text` arm for `spawned`/`exit`/`error`
     parsing — those stay JSON.
   - Input: send `Message::Binary([0x01, payload])`. Resize/kill remain
     `Message::Text` JSON.
2. **`types/terminal.rs`** — no enum change needed for the wire (frames are
   handled before serde), but document the binary framing next to
   `TerminalServerMessage`.
3. **Verify:** run a terminal tab, `cat` a multi-MB file, confirm output
   throughput improves and there are no regressions in resize/exit paths.

## Phase 3 — Mobile (`apps/mobile`)

1. **`hooks/useTerminal.ts` / `useTerminalScreen.ts`**
   - Append `&proto=binary` to the WS URL.
   - Set `ws.binaryType = "arraybuffer"` before connecting (React Native
     supports `arraybuffer` for WS binary frames).
   - `onMessage`: for `typeof data === "string"`, parse JSON as today
     (`spawned`/`exit`/`error`); for `ArrayBuffer`, check
     `new Uint8Array(data)[0] === 0x01` and feed the payload into the same
     sink the `output` JSON event uses today (decode UTF-8 once — `TextDecoder`
     is available via polyfill in RN, or the existing grid sink if it accepts
     bytes).
   - Input: send `Uint8Array` with tag byte; keep resize/kill as JSON strings.
2. **`types/terminal.ts`** — re-export the updated `TerminalSpawnParams` from
   `@console/types`; add a short comment that output may arrive as binary.
3. **Verify:** open a terminal on the Android build, run `ls -la /` and a
   long-running build, confirm output streams and input/resize still work.

## Rollout order & safety

- Server ships first with the `proto` negotiation — zero client breakage is
  possible because old clients never opt in.
- Desktop and mobile flip independently; each can be reverted by dropping the
  query param without a server redeploy.
- The JSON path is never deleted in this plan. Deleting it (a later cleanup
  once both clients have shipped for a while) is a separate, optional task.

## Non-goals / explicitly not now

- Per-message WebSocket compression (CPU cost beats the win for terminal data).
- Multi-terminal multiplexing on one socket.
- Changing coalescing timing (`OUTPUT_FLUSH_MS` / `OUTPUT_FLUSH_BYTES`).
