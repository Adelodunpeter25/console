# Device Streaming Performance Audit & Plan

Scope: the device (iOS simulator / Android emulator) live streaming path in this repo
(`apps/server-go`, desktop `console-ui/devices`, Android `feature/devices`) and the
sim-go SDK (`/Users/adelodunpeter/Developer/Projects/sim-go`).

Pipeline: device H.264 (scrcpy on Android, idb_companion on iOS) -> AVCC conversion ->
per-device session hub -> per-viewer queue -> WebSocket -> WebCodecs canvas player.

Note: findings come from reading code only; no benchmarks were run. Items marked
"verify" are suspected, not confirmed.

## Highest impact

1. **Lag builds up for slow viewers.** `sdk/stream.go` `packetBuffer = 240` (~4-8s of
   video) before any dropping starts, and queued stale frames still play after a drop.
   Shrink to ~5-10 frames. On overflow: flush the queue, wait for a keyframe, call
   `reset()`.
2. **iOS drops raw video chunks, corrupting the picture.** `internal/idb/video.go` pump
   does a non-blocking send into a 32-chunk channel and drops chunks when full. Chunks
   are arbitrary slices of an Annex-B stream, so a drop can cut a NAL in half (smearing
   or decoder errors until the next, rare, keyframe). Apply backpressure, or drop whole
   pictures after the assembler instead.
3. **iOS touch isn't live.** No move events: `releaseGesture` replays the drag as one
   0.2s swipe on release. Scroll is a 0.1s swipe; each opens a new gRPC `Hid` stream and
   runs synchronously in the reader goroutine, so bursts queue for seconds. Use real
   down/move/up on one long-lived HID stream, or coalesce scroll events (latest wins).
4. **Late joiners on iOS may never get a keyframe (verify).** `hasActiveViewers()` uses
   `activeViewers() > 1`, which counts the joiner, so `reset()` skips the pipe restart
   when one viewer already exists. Fix for both platforms: cache the last keyframe plus
   deltas since it in the session and replay on attach.
5. **Fixed quality, no adaptation.** iOS: scale 1.0, 30fps, no `avg_bitrate`. Android:
   `max_size=1280`, 60fps, 6 Mbps. Mobile/remote clients get the same settings as
   loopback desktop. Let the viewer pass `scale`, `max_fps`, `bitrate` as query params;
   mobile/remote defaults around iOS scale 0.5-0.6, Android 720p at 2-3 Mbps.

## Server per-frame cost

- Per-viewer copy of every frame: `tagged()` (sim-go) and `writePacket`
  (`services/devices.go`) allocate `1+len(data)` per viewer per frame. Build the tag once
  (reserve a header byte in the backend buffer) or write tag + payload via `NextWriter`.
- Android `ReadFrame` allocates header + payload per frame on an unbuffered socket. Use a
  64 KB `bufio.Reader` and a reused header. `ToAVCC`/`splitNALs` scan byte by byte and
  grow with `append`: use `bytes.Index`, pre-size output.
- iOS assembler: `append(a.buf, chunk...)` per chunk, byte-wise start code scan,
  `avcc()` allocates per picture, one-frame hold-back (~33ms at 30fps). Use
  `bytes.Index` and a buffer pool.
- `broadcast` builds a new handles slice per frame: use a copy-on-write snapshot.
- Android `SendRaw` holds `ctrlMu` with no write deadline; a stalled adb tunnel blocks
  the reader goroutine and all later input.

## Correctness issues that hurt responsiveness

- **Android rotation / resolution change:** session `width`/`height` and the `Meta` used
  by `EncodeTouch` are set once; new scrcpy configs don't update them, so touch maps
  wrongly and viewers' meta is stale. Parse the new size and re-publish desc + meta.
- **30s idle teardown:** leaving the tab longer means a full cold start (adb `stat`,
  `forward`, `app_process`, 150ms `dialVideo` polling). Raise idle to ~2-5 min or hold
  while backgrounded; parallelize stat/forward/launch.

## Client player

Duplicated between the server-served desktop page (`/api/devices/player`) and
`android/.../DevicesPlayerHtml.kt`.

- No decoder backlog control: if `decoder.decodeQueueSize` exceeds ~3-5, skip to the next
  keyframe and send `{type:'reset'}`.
- `fitCanvas()` runs on every decoded frame (forces layout): call only on size change.
- Draw once per `requestAnimationFrame` using only the latest `VideoFrame`; close older.
- `getContext('2d', {alpha:false, desynchronized:true})`.
- Coalesce `pointermove` to one message per animation frame; skip when
  `ws.bufferedAmount` is high.
- Decoder config: `hardwareAcceleration:'prefer-hardware'`; reconfigure instead of
  destroying on a new description with the same SPS.
- Reconnect: exponential backoff instead of fixed 2s; reuse the cached keyframe (item 4)
  so reconnects show a picture immediately.
- Share one player source between desktop and Android so fixes don't drift.

## Suggested order

1. Queue length, flush, keyframe resync (items 1 and 4).
2. iOS chunk-drop fix and keyframe cache for late joiners (items 2 and 4).
3. Player: decoder backlog guard, rAF drawing, `fitCanvas` fix, move coalescing.
4. Quality parameters and mobile defaults.
5. iOS live touch and scroll coalescing.
6. Allocation cleanups (matter at 60fps or multiple viewers).

Before step 6, add timing logs (capture -> send, send -> decode, decode -> draw) to see
whether latency is server-side or in the player.
