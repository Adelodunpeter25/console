# QUIC / HTTP-3 + WebTransport Migration Plan

This is the migration plan for moving the coding agent harness server from HTTP, SSE (chat streaming), and standard TCP WebSocket (binary interactive terminal) to HTTP/3 and WebTransport using QUIC.

Current baseline:

* `apps/server-go/internal/serve/serve.go:119`: Fiber `app.Listen(:3000)`.
* `apps/server-go/internal/routes/sse.go:82` + `run.go:70`: `streamSSE()` for chat/fs/git.
* `apps/server-go/internal/routes/terminal.go:55`, `ports.go:106`, `devices.go:118`: `fasthttp/websocket` binary terminal (`INPUT_FRAME_TAG`/`OUTPUT_FRAME_TAG`), port tunnel, device stream.
* `apps/android/.../data/stream/ChatStreamClient.kt:73`: OkHttp SSE `callbackFlow`.
* `apps/android/.../data/repo/TerminalRepository.kt:44`: OkHttp `newWebSocket()` terminal.
* `apps/android/.../data/api/HttpTransport.kt:15`: OkHttp unary REST.
* `apps/desktop/crates/console-core/src/utils/sse_reader.rs`, `http_transport.rs`, `services/terminal/mod.rs:137`, `local_forward.rs:159`: `reqwest + eventsource-stream + tokio-tungstenite + tokio::spawn`.

### Phase 0 — Audit + abstraction seam

1. Inventory all transports: `run/stream`, `fs/watch`, `git/status/watch`, `ports/stream+tunnel`, `devices/stream`, `terminals`.
2. Extract business logic from Fiber handlers into transport-agnostic `internal/services/transport` interfaces: `RunSubscriber`, `PtySession`, `FsWatcher`. Both Fiber and WebTransport handlers will call these. Do not delete Fiber yet.
3. Add `console.toml` flags: `transport={auto|wt|legacy}`, `wt_url`, `legacy_url`.

Exit: Fiber still serves `:3000`, new code paths behind interface.

### Phase 1 — Wire protocol over one WebTransport session

One WT session per device: `POST https://host:443/wt/v1/connect?token=...` with `Sec-WebTransport-Protocol: console-v1`.

Multiplex inside with QUIC bidi streams, not TCP sockets:

```
CLIENT opens bidi STREAM -> [uvarint stream_kind][uvarint id_len][id][uvarint payload_len][JSON payload...]
SERVER replies on same STREAM, server-push opens new STREAM for same id
```

1. `0x00 CHAT_RUN`: replaces SSE. Request JSON = existing `POST /api/sessions/:id/run` body, response frames = existing `AgentSessionEvent` length-prefixed JSON (`u32BE len + JSON`). Keep `seq/id:` cursor for `?since=` resume.
2. `0x01 TERMINAL`: replaces WS binary. Reuse `TerminalSpawnParams`, `spawned/exit/error/output` JSON + binary tag byte protocol (`INPUT_FRAME_TAG`/`OUTPUT_FRAME_TAG`) inside stream payload to avoid client parser rewrite. One QUIC stream per terminal ID.
3. `0x02 FS_WATCH / GIT_WATCH / PORTS_STREAM`: same SSE JSON, separate streams.
4. `0x03 PORT_TUNNEL`: raw TCP proxied as reliable stream, preserves `ports.go:100` semantics.
5. DATAGRAMs only for keepalive/ping and ephemeral cursor hints. No chat/terminal data loss path on datagrams.

Auth: WT handshake cannot send custom `Authorization` reliably — pass short-lived token in query + validate before `Accept()`, then bind `device_id` to `quic.Connection`.

### Phase 2 — Server: `quic-go` + `webtransport-go` alongside Fiber

1. Add `internal/wt/server.go`: `quic-go >=v0.48` + `quic-go/http3` + `quic-go/webtransport-go`. `http3.Server{Addr::443, TLSConfig, QUICConfig}`.
2. `QUICConfig`: `KeepAlivePeriod:15s, MaxIdleTimeout:60s, EnableDatagrams:true, MaxIncomingStreams:1000, MaxIncomingUniStreams:1000`.
3. TLS: real cert for `:443` (LetsEncrypt). Local dev: `mkcert` + `console.toml: insecure_skip_verify` only for dev.
4. Dual-listener: keep Fiber TCP `:3000` (or `:443` TCP via `tls.Listener` multiplex), new `udp.ListenPacket(:443)` for QUIC. Same process, different sockets — no `SO_REUSEPORT` conflict (TCP vs UDP).
5. `Alt-Svc: h3=":443"` on Fiber responses to advertise H3.
6. Graceful shutdown: extend `serve.go:132` `ShutdownWithTimeout` to `wtServer.CloseGracefully()` + `shutdownAll()` (PTYs, runs).
7. qlog + `GSO/GRO`: enable `QUIC_GO_ENABLE_GSO=true`, bump `SO_RCVBUF/SNDBUF` to 4-8MB, `sync.Pool` for `[]byte` in terminal hot loop.

Exit: `curl --http3` + WT echo works, legacy clients unaffected.

### Phase 3 — Android: Cronet for WT, OkHttp stays for fallback

OkHttp has no WebTransport. Keep `HttpTransport.kt`, `ChatStreamClient.kt`, `TerminalRepository.kt` interfaces, swap internals.

1. Dep: `org.chromium.net:cronet-embedded:108+`, init `CronetEngine` in `ConsoleApplication.kt` / `AppContainer.kt` with `enableQuic(true)`, `enableHttpCache(DISK,100MB)`, `enableConnectionMigration(true)`, `addQuicHint(host,443,443)`.
2. New `data/wt/CronetWtSession.kt`: `engine.newBidirectionalStreamBuilder(url, callback, executor).setHttpMethod("CONNECT") + :protocol: webtransport`. Implement `onStreamReady -> openBidiStream(kind,id)` mapping to Phase-1 framing.
3. `ChatStreamClient`: add `startRunWt()` returning same `Flow<StreamOutcome>`; parse `u32BE+JSON` instead of `data:` lines. `RunStreamController` unchanged.
4. `TerminalRepository`: replace `httpClient.newWebSocket()` with `wtSession.openTerminalStream(params)`; keep `INPUT_FRAME_TAG` framing so `TerminalStateHolder.appendOutput` path is identical. Cronet callbacks arrive on background executor — forward to `scope (Main.immediate)` via `StateFlow`.
5. Racing fallback: `WtProbe`: try WT `CONNECT` with 300ms timeout; on `QUIC_HANDSHAKE_TIMEOUT / ERR_QUIC_PROTOCOL_ERROR` set sticky `useLegacy=true` per `Network` (register `ConnectivityManager.NetworkCallback`). Unary REST always via OkHttp/Cronet `UrlRequest`.

### Phase 4 — Desktop Rust: `wtransport` + Quinn off UI thread

GPUI constraint: 120fps render loop must never `.await` network.

1. Dep: `wtransport = { version="0.3", features=["tokio-runtime","rustls"] }` (Quinn under the hood), `quinn` for tuning. Keep existing `tokio` runtime already in `apps/desktop/Cargo.toml:39`.
2. New `console-core/src/net/wt_client.rs`: `WtClient { endpoint, connection: Arc<Mutex<WebTransportClient>> }` with `connect(url).await`, `open_bidi(kind,id) -> (SendStream,RecvStream)`.
3. Pattern per service (`sse_reader.rs`, `terminal/mod.rs`, `local_forward.rs`):
   ```rust
   cx.background_executor().spawn(async move {
     let (tx, rx) = tokio::sync::mpsc::channel::<Frame>(1024);
     tokio::spawn(wt_read_loop(stream, tx)); // Quinn poll
     while let Some(f) = rx.recv().await {
       model.update(&mut cx, |m, cx| { m.apply(f); cx.notify(); }) // marshal to UI
     }
   }).detach();
   ```
   Never `block_on` in `view.rs`; coalesce terminal output with existing `STREAM_RENDER_INTERVAL` debounce in `src/state/run.rs:30` and `terminal_view/mod.rs:188`.
4. Fallback: `TransportSelector::race()`: `tokio::select!{ wt= WtClient::connect_with_timeout(500ms) => Wt, _ = sleep => Legacy(reqwest SSE + tungstenite) }`. Cache decision for session.
5. TLS: `rustls-native-roots`; local dev flag for self-signed.

### Phase 5 — TCP fallback / corporate firewall traversal

1. Same `:443` dual-stack: TCP `:443` (Fiber/TLS) + UDP `:443` (H3). Firewall blocking UDP still allows TCP.
2. Client logic (both Kotlin/Rust): `Happy Eyeballs`: start WT + legacy in parallel; first success wins; if WT fails with UDP-block signature, pin `legacy` for 5min, retry WT on `NetworkCapabilities` change.
3. Server `Alt-Svc` + client `RequiresUDPProbing`: do not hard-fail offline WT — always have legacy path.

### Phase 6 — Connection migration (mobile)

1. Server `quic-go`: do not bind `ConnectionID` to 4-tuple; set `StatelessResetKey`, disable `DisablePathMTUDiscovery=false`, `Allow0RTT:false` initially (0-RTT after stable). Behind LB: need CID-aware routing.
2. Android Cronet: `enableConnectionMigration(true)`, handle `onFailed` with `NETWORK_CHANGED` -> `wtSession.migrate()` not full reconnect; preserve `since seq` cursor for chat resume.
3. Rust: `quinn::TransportConfig{ keep_alive_interval, max_idle_timeout, migration:true }`, subscribe to OS network change (GPUI `background_executor` timer + socket rebind test).
4. Test matrix: WiFi->LTE, LTE->WiFi, VPN on/off, NAT rebinding. Assert no terminal kill, chat resumes from `seq`.

### Phase 7 — Perf / profiling for user-space UDP

QUIC cost is syscalls + allocations, not bandwidth.

1. Server: `go pprof + quic-go qlog + netstat -su`: track `udp read errors`, retransmits, `GSO` batch size. Tune `MaxIncomingStreams`, `InitialStreamReceiveWindow (512KB terminal)`, `ConnectionReceiveWindow (2MB)`.
2. Pool buffers: replace per-frame `make([]byte)` in terminal relay with `sync.Pool`; enable `GSO/GRO`; raise `net.core.rmem_max/wmem_max`.
3. Android: Perfetto + Cronet `NetLog`; watch binder hops — keep parsing off main thread, reuse `Json` instance.
4. Desktop: `tokio-console` + `cargo flamegraph` on `wt_read_loop`; bounded `mpsc(1024)` backpressure drops coalesced terminal frames, never unbounded `Vec`.
5. Extend `apps/server-go/cmd/bench/main.go` with `wt-fanout N=500 streams` bench: p50/p99 frame latency, CPU/alloc per stream.

### Phase 8 — Rollout

1. `v1`: server dual-serve, clients `auto` (WT try, legacy fallback), feature-flagged.
2. `v2`: WT default, legacy only on UDP-block.
3. `v3`: deprecate WS/SSE (keep unary REST over H3).

Risks: Fiber/`fasthttp` has no H3 — you must run separate `http3.Server`; Cronet APK +15MB; `wtransport` vs GPUI `smol` executor needs explicit `tokio` bridge.
