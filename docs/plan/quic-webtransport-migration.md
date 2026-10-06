# QUIC / HTTP-3 + WebTransport Migration Plan

Migration plan for moving the coding agent harness server from HTTP, SSE (chat
streaming), and standard TCP WebSocket (binary interactive terminal) to HTTP/3
and WebTransport over QUIC.

Current baseline (line numbers verified against the tree):

* `apps/server-go/internal/serve/serve.go:119` — Fiber `app.Listen(addr)`, default `:3000`, overridable by `PORT`.
* `apps/server-go/internal/routes/sse.go:84` — `streamSSE()`; `sseStream` with `Send`/`SendJSON`/`SendSeqJSON`/`Ping`.
* `apps/server-go/internal/routes/run.go:48` (`POST /api/sessions/:id/run`), `run.go:78` (`GET .../run/stream`), `run.go:294` `pumpHub`, `run.go:360` `sendFrame`.
* `apps/server-go/internal/routes/terminal.go:61` — websocket upgrade; `terminal.go:71` `handleTerminalConn`; `terminal.go:41` `terminalFramePool` (already a `sync.Pool`).
* `apps/server-go/internal/routes/ports.go:68` (`/api/ports/stream`), `ports.go:153` (`/api/ports/:port/tunnel`).
* `apps/server-go/internal/routes/devices.go:112` (`/api/devices/:id/stream`), `devices.go:135` `deviceWSConn` → `services.StreamConn`.
* `apps/server-go/internal/routes/misc.go:25` (`/api/notifications/stream`), `apps/server-go/internal/routes/scripts.go:145` (`/api/projects/:id/scripts/runs/:runId/stream`).
* `apps/server-go/internal/routes/routes.go:106` — `shutdown()` (runs, bash jobs, PTYs, scripts, fff, mcp, devices).
* `apps/server-go/internal/serve/serve.go:136` — `app.ShutdownWithTimeout(5s)` followed by `shutdownAll()`.
* `apps/android/app/src/main/kotlin/com/console/mobile/data/stream/ChatStreamClient.kt:73` — OkHttp SSE `callbackFlow`.
* `apps/android/app/src/main/kotlin/com/console/mobile/data/repo/TerminalRepository.kt:221` — `httpClient.newWebSocket(...)`; `:67` Wire+Moshi adapters; `:187` `onFailure`; `:198` `onClosed`.
* `apps/android/app/src/main/kotlin/com/console/mobile/data/api/HttpTransport.kt:15` — OkHttp unary REST.
* `apps/android/app/src/main/kotlin/com/console/mobile/AppContainer.kt:163` — `initialize()`, builds `httpCallClient`/`httpClient`; no Cronet, no `ConsoleApplication.kt`.
* `apps/desktop/crates/console-core/src/utils/sse_reader.rs`, `http_transport.rs`, `services/terminal/mod.rs:291` (`connect_async`), `services/local_forward.rs:167` (`accept_loop`) — `reqwest + eventsource-stream + tokio-tungstenite + tokio::spawn`.
* `apps/desktop/Cargo.toml:44` `reqwest`, `:46` `tokio`, `:48` `tokio-tungstenite`.

### Wire format note (drives Phase 1 effort)

The existing wire is **protobuf-JSON (`protojson`), not plain JSON**. Terminal
control frames (`terminal.go:96-116`), fs (`fs.go:76`), git (`git.go:44`), and
ports (`ports.go:57`) all marshal generated `console.v1` messages through
`protojson`; Android decodes them with Wire + Moshi oneof adapters
(`TerminalRepository.kt:67-68`) and desktop with `serde_json` of
`TerminalServerMessage`. Only `modelStreamPartFrame` (`run.go:372`) is a
hand-written struct. Any WT wire must pick one of:

1. **Recommended** — add `console.v1` messages for the WT envelope
   (`WtRequest`/`WtResponse`) and reuse generated types for payloads. One
   encoding, generated clients on all three platforms, parity with the existing
   `proto/console/v1/*.proto` workflow.
2. Define a raw JSON envelope and accept two codecs in the codebase (protojson
   today, plain JSON over WT). Smallest schema work, largest client churn.

Decide this before Phase 1 coding. Binary tag-byte framing
(`INPUT_FRAME_TAG`/`OUTPUT_FRAME_TAG`) rides *inside* terminal payloads and stays
intact either way.

### Auth gap (cross-cutting, blocks Phase 1)

The server has **no request bearer auth today**. `routes.go` installs only
`recover`; `internal/auth` is provider OAuth + GitHub PAT state, unrelated to
API auth. Clients send `Authorization: Bearer` opportunistically
(`HttpTransport.kt:39-40`, `ChatStreamClient.kt:78`,
`TerminalRepository.kt:84-86`) and nothing validates it.

Phase 1's "short-lived token in query, validate before `Accept()`, bind
`device_id` to the connection" is therefore greenfield. Add before the WT
listener exists:

* token issuer + validator (HMAC-signed, TTL, single-session revocation),
* an endpoint clients can mint one from over the existing REST surface,
* a middleware that accepts either `Authorization` or the query token so the
  WT path and the legacy path share one validator.

### Phase 0 — Audit + abstraction seam

1. Full transport inventory — the seven streaming/bidirectional endpoints, not
   the four named in the first draft:

   | Endpoint | Kind | Resume cursor | Client |
   |---|---|---|---|
   | `POST /api/sessions/:id/run` | SSE + POST | `id:` seq | Android, desktop |
   | `GET /api/sessions/:id/run/stream` | SSE | `?since=` seq | Android, desktop |
   | `GET /api/fs/watch` | SSE | none | desktop |
   | `GET /api/git/status/watch` | SSE | none | desktop |
   | `GET /api/ports/stream` | SSE | none | desktop |
   | `GET /api/notifications/stream` | SSE | none | Android |
   | `GET /api/projects/:id/scripts/runs/:runId/stream` | SSE | none | desktop |
   | `GET /api/terminals` | WS binary | none | Android, desktop |
   | `GET /api/ports/:port/tunnel` | WS raw TCP | none | desktop |
   | `GET /api/devices/:id/stream` | WS framed | pong TTL | desktop |

   Unary stays on REST: `GET /api/fs/file/raw` (image/SVG bytes + ETag),
   `GET /api/devices/:id/screenshot` (PNG), and every other `/api/*` route.

2. Extract transport-agnostic logic behind interfaces. Most of the seam already
   exists — do not rebuild it:

   * `run.Hub.Subscribe(since)` / `Unsubscribe` (`run.go:295-300`) is the
     transport-free run API.
   * `services.StreamConn` + `PumpStream` (`devices.go:135-173`) already
     abstracts WS behind an interface. Use it as the template for the WT
     adapter, and reuse it for the device stream rather than adding a parallel
     one.
   * `PtyManager`, `PortRegistry`, `FsWatchService` are already transport-free.

   Remaining work: introduce `internal/services/transport` with `RunSubscriber`,
   `PtySession`, `FsWatcher` only where Fiber handlers still own logic
   (`fs.go:308-342`, `git.go:65-101`, `ports.go:68-100`, `misc.go:25`,
   `scripts.go:145` all inline their pump loops inside the handler).

3. Config flags. `console.toml` is a **desktop run-scripts file only**
   (`[scripts.*]`); it is not read by the server or the clients. So:

   * server: new env/flag surface (`CONSOLE_TRANSPORT=auto|wt|legacy`,
     `CONSOLE_WT_ADDR`, TLS cert/key paths) alongside the existing `PORT`;
   * Android: keys in `PreferencesStore`;
   * desktop: keys in the existing backend/environment settings model
     (same shape as `ConsoleApiClient.baseUrl`).

   `transport`, `wt_url`, `legacy_url` as *client-side* concepts; the server
   only needs "am I serving H3" plus listener addresses.

Exit: Fiber still serves `:3000`, all streaming logic reachable through
transport-agnostic interfaces, token issuer/validator in place.

### Phase 1 — Wire protocol over one WebTransport session

One WT session per device. The handshake is **`CONNECT`**, not `POST`:

```
CONNECT /wt/v1/connect?token=... HTTP/3
:protocol: webtransport
sec-webtransport-http3-draft: draft02
```

(Exact draft header follows whatever `quic-go/webtransport-go` pins; take it
from the library's own example rather than hardcoding.)

Multiplex inside with QUIC bidi streams, not TCP sockets. Pick **one** framing —
do not ship the two schemes in the first draft (a `[uvarint kind][uvarint
id_len][id][uvarint len][JSON]` header plus `u32BE len + JSON` bodies means two
parsers and two sets of tests on three platforms):

```
STREAM header (once): [uvarint kind][uvarint id_len][id]
STREAM body:         repeated length-prefixed frames
FRAME:               [u32BE len][protojson(WtFrame)]   // console.v1 WtFrame
```

Stream kinds:

1. `0x00 CHAT_RUN` — replaces both run SSE endpoints. Request payload = the
   existing `runPromptBody` (`run.go:19`) for start, or `{since}` for attach.
   Response frames carry `AgentSessionEvent` equivalents **plus an explicit
   `seq` field per frame**, since the `id:` SSE line (`sse.go:58`) has no
   equivalent once you are length-prefixing.
2. `0x01 TERMINAL` — replaces WS binary. Reuse `TerminalSpawnParams`
   (`terminal.go:245`) and the `spawned/exit/error/output` protobuf oneof, with
   the binary tag byte (`OUTPUT_FRAME_TAG`/`INPUT_FRAME_TAG`) inside payload
   bytes so `TerminalStateHolder.appendOutput` and the desktop's
   `advance_and_collect_replies_bytes` paths stay identical. One stream per
   terminal id.
3. `0x02 FS_WATCH / GIT_WATCH / PORTS_STREAM / NOTIFICATIONS_STREAM /
   SCRIPT_RUN_STREAM` — same event payloads, separate streams, each identified
   by its query-equivalent request params (`path`, `host`, `projectId`,
   `projectId`, `runId`).
4. `0x03 PORT_TUNNEL` — raw TCP proxied as a reliable stream, preserving
   `ports.go:153-211` semantics including the 500ms drain on half-close.
5. `0x04 DEVICE_STREAM` — reuse `services.StreamConn` via an adapter instead of
   reimplementing the framing.
6. DATAGRAMs: **disabled for v1**. Keepalive comes from `QUICConfig.KeepAlivePeriod`
   and the existing 15s app-level pings; there is no datagram payload worth the
   extra PMTU handling. Re-evaluate only if a real loss-tolerant channel appears.

**Side-effecting control ops stay unary REST for v1.** `abort`, `answer`,
`approve`, `browser-action`, `queue`, `steer` (`run.go:116-256`) and terminal
`resize`/`kill` (`terminal.go:226-237`) are idempotent-ish and already exist as
routes. Moving them onto WT streams means the Happy-Eyeballs race in Phase 5
could double-fire them. Keep them REST in v1; add them as request frames over
the session in v2 once one transport is authoritative.

Resume semantics differ per stream and the plan must say so: only `CHAT_RUN`
has a cursor today (`sse.go:50-68`, `run.go:294-306`). `fs/watch`,
`git/status/watch`, `ports/stream`, `notifications/stream` and the script stream
are snapshot-per-event with no replay buffer — a dropped WT session means a fresh
snapshot on reconnect, which is already their current behavior.

Auth: validate the query token before `Accept()` (see the Auth gap section), then
bind `device_id` to the `quic.Connection`. Reject with an HTTP status before the
session is established so clients can distinguish "bad token" from "UDP blocked".

### Phase 2 — Server: `quic-go` + `webtransport-go` alongside Fiber

1. `internal/wt/server.go`: latest `quic-go` compatible with `go 1.25.0`
   (`go.mod:3`) plus matching `quic-go/http3` and `quic-go/webtransport-go`.
   Resolve the triple together — `webtransport-go` pins a `quic-go` minor, and
   mixing majors fails at compile time.
   `http3.Server{Addr: ":443", TLSConfig: ..., QUICConfig: ...}` (Go syntax).
2. `QUICConfig` on day one, not deferred to Phase 7:

   ```go
   &quic.Config{
       KeepAlivePeriod:        15 * time.Second,
       MaxIdleTimeout:         60 * time.Second,
       EnableDatagrams:        false,
       MaxIncomingStreams:     1000,
       MaxIncomingUniStreams:  1000,
       InitialStreamReceiveWindow:     512 * 1024,  // terminal stream
       ConnectionReceiveWindow:        2 * 1024 * 1024,
   }
   ```

   `InitialStreamReceiveWindow` at its default is too small for terminal output
   bursts — without it the echo test in Phase 2 stalls and it looks like a QUIC
   bug.
3. TLS: real cert on `:443`. Today Fiber serves **plaintext HTTP**, so
   `Alt-Svc` (step 5) has nothing valid to advertise and `Sec-WebTransport-*`
   cannot work at all. Sequence it:

   a. add TLS to the Fiber listener (so TCP `:443` speaks HTTPS and can serve
      `Alt-Svc`);
   b. serve H3 on UDP `:443`.

   Dev trust: `mkcert` for the server, plus Android emulator/device CA trust via
   `network_security_config` and desktop `rustls-native-roots` loading the mkcert
   root. `insecure_skip_verify` only as a last-resort dev flag.
4. Dual listener: Fiber on TCP `:443` (TLS) and `udp.ListenPacket(":443")` for
   QUIC. Same process, different sockets — TCP vs UDP, no `SO_REUSEPORT` needed.
   Keep `:3000` plaintext as the dev path so existing local setups keep working.
5. `Alt-Svc: h3=":443"` on Fiber responses, only once TLS is on (step 3a).
6. Graceful shutdown: in `serve.go:136`, extend the existing sequence with
   `wtServer.CloseGracefully()` **before** `app.ShutdownWithTimeout` and the
   existing `shutdownAll()` (`routes.go:106`) so PTYs and in-flight runs drain
   through both paths.
7. qlog behind a flag; GSO/GRO + socket buffers in Phase 7 (needs sysctl
   privileges, not a Go env var alone).

Exit: `curl --http3` and a WT echo both work; legacy clients unaffected.

### Phase 3 — Android: Cronet for WT, OkHttp stays for fallback

OkHttp has no WebTransport. Keep `HttpTransport.kt`, `ChatStreamClient.kt`, and
`TerminalRepository.kt` interfaces; swap internals.

1. Dep `org.chromium.net:cronet-embedded` matching `compileSdk 36` /
   `minSdk 26` (`app/build.gradle.kts:28-30`). Size: the +15MB estimate is per
   ABI, and `abiFilters` currently ships four (`build.gradle.kts:35-37`) — use an
   App Bundle ABI split, or keep Cronet behind a `wt` build flavor until the
   handshake is stable.
2. Init in `AppContainer.kt:163 initialize()`, next to the existing
   `httpCallClient`/`httpClient` pair (`:167-180`) — there is no
   `ConsoleApplication.kt`. Builder: `enableQuic(true)`,
   `enableHttpCache(DISK, 100MB)`, `enableConnectionMigration(true)`,
   `addQuicHint(host, 443, 443)`.
3. `data/wt/CronetWtSession.kt`: `newBidirectionalStreamBuilder(url, callback,
   executor)` with `setHttpMethod("CONNECT")` + `:protocol: webtransport`, then
   map `onStreamReady` onto the Phase-1 framing. Verify the exact
   `sec-webtransport-http3-draft` header against the Cronet version in use —
   Cronet's supported drafts have moved over releases.
4. `ChatStreamClient`: add `startRunWt()` returning the same
   `Flow<StreamOutcome>` and same `SseStreamFrame(seq, event)` shape, so
   `ChatRepository` and `RunStreamController` stay untouched — only the reader
   changes from line-splitting `data:` to length-prefixed frames, with `seq` read
   from the frame field instead of the `id:` line.
5. `TerminalRepository`: swap `newWebSocket` (`:221`) for
   `wtSession.openTerminalStream(params)`. Keep the tag-byte framing so
   `terminalState.appendOutput` (`:162`, `:182`) is unchanged. Cronet delivers
   callbacks on a background executor, so forward through the existing
   `Dispatchers.Main.immediate` scope (`:47`) via `StateFlow` rather than
   touching holders from the callback thread. Preserve the `onClosed`-before-
   `spawned` deferred cleanup (`:198-215`) — it is a real bug fix, not optional.
6. Racing fallback (`WtProbe`): **300ms is too tight** for a QUIC handshake on
   LTE — it produces false negatives that pin the app to legacy. Use 1500-2500ms,
   or better, probe on a background `NetworkCallback` and cache per `Network`
   via `ConnectivityManager`. On `QUIC_HANDSHAKE_TIMEOUT` /
   `ERR_QUIC_PROTOCOL_ERROR` / `ERR_QUIC_HANDSHAKE_FAILED`, set sticky
   `useLegacy=true` for that `Network`. Unary REST stays on OkHttp/Cronet
   `UrlRequest`.

### Phase 4 — Desktop Rust: `wtransport` off the UI thread

GPUI constraint: the render loop must never `.await` network.

1. Dep: `wtransport` at a version whose `quinn`/`rustls` resolve cleanly against
   `reqwest 0.12` with `rustls-tls` (`Cargo.toml:44`) and the pinned gpui toolchain.
   A 0.3-era `wtransport` will conflict — check `cargo tree` for two `rustls`
   majors before writing any client code. Keep the existing `tokio` (`Cargo.toml:46`).
2. `console-core/src/net/wt_client.rs`: `WtClient` holding endpoint +
   connection, `connect(url)`, `open_bidi(kind, id) -> (SendStream, RecvStream)`,
   `close()`. It must stay free of `cx` — see step 3.
3. Layering: `console-core` never sees `Context`/`Entity`, so the snippet below
   cannot live in `wt_client.rs`. Keep `WtClient` in `console-core` and do all
   GPUI marshaling in `apps/desktop/src` (`src/state/run.rs` and the terminal
   view):

   ```rust
   // apps/desktop/src — background_executor task
   let (tx, mut rx) = tokio::sync::mpsc::channel::<Frame>(1024);
   let read = tokio::spawn(wt_read_loop(stream, tx));
   while let Some(f) = rx.recv().await {
       model.update(&mut cx, |m, cx| { m.apply(f); cx.notify(); });
   }
   ```

   Never `block_on` in a view. Keep the existing coalescing discipline —
   `TerminalHandle::scroll` (terminal/mod.rs:148-195) already coalesces on a 4ms
   delay, and `notify.notify_one()` drives the redraw; mirror that for WT reads
   rather than adding a second timer.
4. Fallback: `TransportSelector::race()` with a `tokio::select!` between
   `WtClient::connect_with_timeout` and a legacy `reqwest`/tungstenite path.
   **500ms is too short** for a real QUIC handshake; use the same 1500-2500ms as
   Android. Cache the decision for the session. Make sure racing cannot
   double-spawn: the WT probe must be a bare handshake, and the real connect
   happens only on the winning branch.
5. TLS: `rustls-native-roots`, plus mkcert root loading for local dev.

### Phase 5 — TCP fallback / firewall traversal

1. Same `:443` dual stack: TCP `:443` (Fiber/TLS) + UDP `:443` (H3). A firewall
   blocking UDP still leaves TCP working.
2. Happy Eyeballs on both clients: start WT and legacy in parallel, first success
   wins. Pin `legacy` for 5 minutes on a UDP-block signature, retry WT on
   `NetworkCapabilities` change. Because Phase 1 keeps side-effecting ops on
   unary REST, the race can only duplicate *connects*, not PTY spawns or runs —
   but do not race the run-start or spawn request itself.
3. `Alt-Svc` on the server plus a client probe that never hard-fails offline WT.

### Phase 6 — Connection migration (mobile)

1. Server: leave `ConnectionID` off the 4-tuple (default), set
   `StatelessResetKey`, enable PMTU discovery, keep `Allow0RTT: false` (run-start
   and PTY spawn are not replay-safe). Behind a load balancer this needs CID-aware
   routing — out of scope while the server is single-box; note it as a
   prerequisite before adding an LB.
2. Android: `enableConnectionMigration(true)`. Cronet does the migration itself;
   there is no `wtSession.migrate()` to call. The work is *recreating* the
   session on failure and resuming from the last `seq` — plus `TerminalRepository`
   must re-open terminals explicitly, since a PTY tied to a dead session cannot
   be migrated.
3. Rust: tune `quinn::TransportConfig` (`keep_alive_interval`,
   `max_idle_timeout`); rebind on OS network change via a
   `background_executor` timer.
4. Test matrix: WiFi→LTE, LTE→WiFi, VPN on/off, NAT rebinding. Assert no
   terminal loss beyond a reported error, and that chat resumes from `seq`.

### Phase 7 — Perf / profiling for user-space UDP

QUIC cost is syscalls and allocations, not bandwidth.

1. Server: `go pprof` + qlog + `netstat -su`. Watch `udp read errors` and
   retransmits. Windows are set in Phase 2; tune here only from measurements.
2. Buffer pooling: `terminal.go` already uses `terminalFramePool`
   (`terminal.go:41-46`) — do not redo it. Remaining allocation sites are the
   per-tunnel `make([]byte, 64*1024)` (`ports.go:169`) and per-frame encodes.
   `GSO`/`GRO` plus `net.core.rmem_max`/`wmem_max` need root sysctl on the host,
   not just `QUIC_GO_ENABLE_GSO=true`; make them a documented host-provision step.
3. Android: Perfetto + Cronet NetLog. Keep parsing off the main thread; reuse
   the `Json` instance (`ConsoleJson`) rather than allocating per frame.
4. Desktop: `tokio-console` and `flamegraph` as dev-only tooling on `wt_read_loop`.
   Bounded `mpsc(1024)` with coalesced drops for terminal frames; never an
   unbounded `Vec`.
5. Bench: do **not** extend `apps/server-go/cmd/bench/main.go` — it measures
   harness token cost per task (`main.go:1-20`), not transport fanout. Add a
   separate `cmd/wtbench` that opens N=500 streams and reports p50/p99 frame
   latency plus CPU and allocations per stream.

### Phase 8 — Rollout

1. `v1`: server dual-serves; clients default to `auto` (try WT, fall back to
   legacy), feature-flagged off by default.
2. `v2`: WT default, legacy only on UDP block. Move side-effecting control ops
   onto the session here.
3. `v3`: deprecate WS/SSE. **Unary REST is never removed** — it is the only
   path for UDP-blocked networks, so Fiber on TCP `:443` stays permanently as the
   fallback plane. Only the streaming endpoints are deprecated.

### Risks

* Fiber/`fasthttp` has no HTTP/3 — a separate `http3.Server` is unavoidable, and
  it doubles the TLS/handshake surface in the same process.
* No request auth exists yet; the token issuer is a prerequisite, not a detail.
* Cronet adds ~15MB per ABI across four `abiFilters`.
* `wtransport` version must be resolved against the pinned `reqwest`/`rustls`
  before client code lands.
* GPUI/smol vs tokio bridge: transport lives in `console-core`, marshaling in
  `apps/desktop/src`.
* Two live transports means every side-effecting route needs an idempotency
  story before it can move onto the session.