# Remote transport optimization — before QUIC

Status: **in progress** (2026-11-16). Items 1, 2 and the connection-pool part of 5 are **implemented**; the rest are sequenced but not scheduled.

## Context

The server is often installed on a remote machine with a much faster connection than the client (1 Gbps vs ~20 Mbps), yet the app feels slower than it does against a server on the same machine — even though both paths go through iroh. Local-through-iroh being fast does not validate the transport; it hides every inefficiency because RTT is ~0 and bandwidth is effectively unlimited. Remote is where the bill comes due.

A previous QUIC/WebTransport stack was built, benchmarked and then removed (Oct–Nov 2026). The iroh tunnel that replaced it deliberately separates two concerns: iroh answers "how do I reach the box" (key identity, NAT traversal, encryption), while plain HTTP over a loopback TCP port answers "how do I talk to the API". This plan optimizes the second half without changing that boundary.

### Correction to an earlier claim

An earlier version of this analysis claimed the loopback bridge shuts down the local TCP socket after each response, forcing a new TCP connection and a new QUIC stream per request. That is **wrong**. With HTTP/1.1 keep-alive, `send.finish()` only fires when reqwest drops the pooled TCP connection, and the server's `HalfClose()` only fires when Fiber closes its side. One QUIC stream is reused per pooled connection across requests. Do not optimize against the false premise.

## What is actually wasteful

Three things scale badly with RTT and link speed:

1. **Bytes on the wire.** Route handlers use `protojson.MarshalOptions{}`. Every `bytes` field (tool arguments, tool results, screenshots, images) is base64-encoded — 33% overhead plus field names — and there is no HTTP compression anywhere in the stack.
2. **Sequential round trips.** Opening one chat fans out into ~8 REST calls (draft, read-report, detail+50 messages, changes, todos, background work, suggestions, queued prompts). Some are parallel, but it is still 3–4 sequential waves before the transcript renders.
3. **Inlined binary.** Images already live on disk per session as `<session>.images/<sha256>`, but they still ride inline as base64 in message payloads and are paid for even when off-screen.

An agent run is dominated by provider token latency, so transport work mostly affects session open, history browsing, and the per-tool round trips — not time-per-token. That distinction should drive measurement.

## Work items

### 1. gzip on the API — [implemented]

- fiber v2 ships the middleware in-tree as `github.com/gofiber/fiber/v2/middleware/compress` — no new module dependency. Installed in `routes.New` just after `recover.New()`.
- SSE is excluded **by path** (`/stream`, `/watch`, and POST `/run`), not by response `Content-Type`: the middleware's `Next` hook runs before handlers set headers, so a content-type check would miss every stream. WebSocket upgrades are unaffected.
- The desktop's reqwest dependency now enables the `gzip` feature. Android's OkHttp already sends `Accept-Encoding: gzip` and decompresses transparently, so it needed no client change.
- No wire or schema change. Expect the largest reduction on text-heavy transcripts (base64'd tool output compresses well; already-compressed image bytes do not).

### 2. Session bootstrap endpoint — [implemented]

- `GET /api/sessions/:id/bootstrap` returns, in one response: session header + first 50 messages (same shape as `GET /api/sessions/:id`), changes-without-diff, todos, suggestions, queued prompts, background work, and the draft. All keys are always present; an absent draft is a literal JSON `null`.
- The handler reuses the existing service methods and proto helpers; no query logic was duplicated. The stale-run status settle from `GET /:id` is mirrored.
- Every existing endpoint is intact — this is additive, so older clients keep working.
- Desktop `load_session_messages_for_pane` now makes one call and feeds the existing state setters from the combined result. `report_session_read` still runs up front; the separate draft poll and the todo/suggestion/queue/background fetches are gone. The stale-response guard (pane/session switch check) covers all bootstrap-fed updates.
- The response type uses `#[serde(flatten)]` over `SessionDetailResponse`. A permanent decode test (`tests/session/bootstrap_dto_test.rs`) pins both the flattened fields and the custom message deserializer.

### 3. Lazy-load images — [not scheduled]

Replace the inline base64 image data in message payloads with a digest reference, served from a dedicated endpoint with `ETag`/`Cache-Control`. The client fetches only images scrolled into view and caches them. Per-endpoint wire change; can be feature-flagged.

### 4. Binary protobuf on heavy endpoints — [not scheduled]

All three clients already generate binary protobuf types (prost, Wire, protoc-gen-go); only the encoding is protojson. Negotiate `application/x-protobuf` via `Accept` on the heavy endpoints (session detail, run events, file content) and use `proto.Marshal` server-side. This removes the base64 overhead and field names without touching the schema or adding a second transport, and can roll out endpoint-by-endpoint with dual decoding during transition. This is the part of the old WebTransport benefit that has nothing to do with QUIC.

### 5. Smaller items

- **OkHttp connection pool — [implemented]**: `AppContainer` now configures `ConnectionPool(16, 5, TimeUnit.MINUTES)` on the shared builder, so both the unary and streaming clients inherit it. The default max idle of 5 was low for REST + SSE + terminal WS + port tunnels. reqwest's pool is fine.
- **Skip iroh locally — [not scheduled]**: when the paired server is on the same machine, use `http://localhost:3000` directly instead of a node-ID tunnel. Cosmetic, since local is already fast.
- **Cache static responses — [not scheduled]**: provider catalog/models already use stale-while-revalidate; check settings, favorites and session list for unnecessary refetches on pane switches.

### 6. Measurement — [not scheduled]

The only recorded number is the tunnel p50/p95 on localhost, which measures tunnel overhead, not the remote problem. Build a bench against a real remote server measuring user-visible time: session-open-to-transcript-rendered, and time-to-first-token on a run, with representative chat payloads. This is the baseline for deciding whether QUIC adds anything after items 1–5.

## Decision gate for QUIC

Revisit native QUIC streams only if items 1–5 are shipped and measured and a gap remains. If so, the shape to use is **iroh for identity/encryption/NAT, native iroh bidirectional streams for the app transport**, reusing the old `wt.proto` framing — not a resurrection of the deleted HTTP/3 + TLS + cert-pinning stack. Keep side-effecting control ops on unary REST in any race-based rollout.

## Shipped in the first pass

| Change | Verification |
|---|---|
| gzip: Fiber middleware + reqwest `gzip` feature | `go build ./...` and `cargo check --manifest-path apps/desktop/Cargo.toml` |
| Bootstrap endpoint (server) | `go test ./tests/api/ -run TestSessionBootstrapRoute -v` |
| Bootstrap client + single-call session open (desktop) | `cargo test -p console-core --test bootstrap_dto_test` (2 tests) |
| OkHttp pool of 16 idle / 5 min | `./gradlew :app:assembleDebug` |

No wire schema changed and no endpoint was removed. The next gate is item 6 — measure against a real remote server before deciding whether any QUIC work is warranted.

## Verification

- Go: `cd apps/server-go && go test ./tests/api/ -run <TestName> -v`, plus `go build ./...`
- Desktop: `cargo check --manifest-path apps/desktop/Cargo.toml` (also `--locked` once the lockfile is committed)
- Desktop contract test: `cargo test --manifest-path apps/desktop/Cargo.toml -p console-core --test bootstrap_dto_test`
- Android: `cd apps/android && ./gradlew :app:assembleDebug`
