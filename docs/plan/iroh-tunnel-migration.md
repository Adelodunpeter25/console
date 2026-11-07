# Replace the relay for device connections with iroh (Phase 1 spike)

Replace the device-facing transport (Cloudflare Worker relay in `apps/relay`,
today's default tunnel backend) with [iroh](https://github.com/n0-computer/iroh)
for the paths where **our own clients** talk to **our own server**: desktop ↔
server, Android ↔ server. The Cloudflare Worker stays untouched as a fallback;
no browser visitors exist today, so nothing else depends on a public HTTPS URL.

## Why

- The relay today forwards every byte through a Cloudflare Worker + Durable
  Object (256 KiB frame cap, per-request latency, rate limits).
- The relayed traffic is not end-to-end encrypted; the README defers an
  encryption layer. iroh encrypts everything always.
- Server identity in the pairing URL (`auth/identity.go` `ServerIdentity`) is a
  courtesy check — the name is only as trustworthy as the relay's
  first-come-first-served registration (`/register` is open). With iroh the
  server's identity is its keypair and the QUIC handshake proves it before any
  pairing traffic flows. Names stay for display; the key is the identity.
- Direct P2P beats the Worker hop for latency in the ~90% of connections that
  hole-punch successfully.

## What iroh replaces — and what it keeps

iroh still uses relays, but only for connection setup and as a fallback
(~10% of connections). Phase 1 uses **n0's free public relays** — no
self-hosting. If/when this ships: n0 paid Shared Relays, or one self-hosted
`iroh-relay` on a VPS (stateless, multi-region failover is built in). Not
Cloudflare — the relay is a long-lived TCP server, not a Worker workload.

## Scope of the change

1. **Tunnel transport** — the core swap. Server dials/accepts iroh connections
   instead of opening an outbound WebSocket to the Worker.
2. **Pairing URL format** — the QR carries the server's node ID (32-byte key)
   + display name instead of a `relay.workers.dev/<name>` base URL. Parsing on
   both ends changes: `apps/desktop/crates/console-shared/src/pairing.rs`,
   `apps/android/.../data/model/PairingUrl.kt`.
3. **Client → server request path** — clients open QUIC streams by node ID on
   an ALPN instead of HTTPS requests to a base URL. Smallest-change approach:
   serve plain HTTP/1.1 over the iroh connection so existing request code
   survives. The tunnel package abstraction (`internal/tunnel/tunnel.go`,
   `cfrelay`, `opentunnel`) gains a third backend rather than replacing the
   other two.
4. **Auth surface** — mostly deletions, not new logic: the client-side
   "does serverId match" advisory check becomes a transport-enforced key
   check. Pairing code → device token flow, TTLs, revoke all unchanged.
5. **Build/packaging** — server uses `iroh-go`
   (git.coopcloud.tech/decentral1se/iroh-go): community FFI bindings, cgo,
   vendored static libs per platform, currently pinning iroh v1.0.3, labeled
   highly experimental. Desktop is Rust (native fit); Android uses the iroh
   Kotlin bindings.

## Phase 1 deliverables (the spike)

Behind the existing backend switch, so the relay keeps working throughout:

- [x] Go server: iroh endpoint via `iroh-go`, listens on an ALPN, serves the
      existing API over HTTP/1.1-on-iroh. Node ID + keypair stored in the
      tunnel dir alongside today's relay info.
- [ ] Desktop: connect to the server by node ID, pairing flow works
      end-to-end over iroh.
- [ ] Android: same, via the Kotlin binding.
- [ ] Pairing QR emits/consumes the new node-ID format on all three ends.
- [x] CF relay backend remains default until the spike proves out.

## Decision gate — measure before shipping

- Hole-punch success rate on real networks (home Wi-Fi, mobile data,
  hotel/office NAT).
- Pairing + request latency vs. the current relay path.
- How painful `iroh-go` CGO cross-compilation is for the platforms the Go
  server ships to (check macOS/Windows static-lib coverage early).
- `iroh-go` stability: if it's too flaky, fall back to a Rust sidecar next to
  the Go server — Android/desktop work carries over either way.

## Later phases

- Make iroh the default device path; CF Worker becomes fallback only.
- Sunset OpenTunnel (`CONSOLE_RELAY=0` path) — redundant once iroh works.
- Optional: self-host one relay + `iroh-dns-server` when production traffic
  justifies it.

## Status

Server side of Phase 1 done and committed locally; nothing is pushed.
Remaining (open):
- [ ] Desktop: connect by node ID
- [ ] Android: connect by node ID
- [ ] Pairing QR node-ID format on all three ends
- [~] Fork `iroh-go` (Adelodunpeter25/iroh-go; upstream rejects contributions
      and lags the latest iroh): forked; still to bump to latest iroh/iroh-ffi
- [x] Fork CI builds static libs for macOS (x64, arm64) and Windows x64;
      Intel mac is link-checked only, never run
- [x] server-go points at the fork; the `iroh` tag now covers linux, macOS
      and windows-amd64. CI for server-go does not build with the tag yet
- [ ] Measure hole-punch rate and latency vs the relay

Built: `internal/tunnel/irohbackend`, behind the `iroh` build tag, selected
with `CONSOLE_RELAY=iroh`. One HTTP request per bidirectional stream on ALPN
`console-device/1`, bridged to the loopback target. Key and node ID live in
the tunnel dir (`iroh.key`, `iroh.json`). Test:
`go test -tags iroh ./tests/tunnel/ -run TestIroh` (uses n0's public relay).

Findings so far (decision gate):
- `iroh-go` has no tags; pinned to a commit. Upstream has static libs for
  linux amd64/arm64 (musl) only; our fork adds macOS and Windows. The backend
  stays behind a build tag until server-go CI builds it on all platforms. Linking the musl lib into a glibc
  build works here.
- The module needs `go 1.26.1` (go.mod bumped from 1.25.0) and the module
  download is ~97 MB (it vendors the libs).
- Stream EOF is `Read` returning (0 bytes, nil) after the peer's `Finish()`.
- A fresh endpoint is only dialable after `Online()`; a client that dials
  before its own `Online()` fails intermittently with an opaque `IrohError`.
- `Online()` blocks forever with relays disabled; direct dial by address
  works without it.
- Errors are opaque; mismatched ALPN gives a bare `IrohError`.
- Not yet measured: hole-punch rate on real networks, latency vs the relay.
