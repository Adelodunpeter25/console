# Computer Use via Cua Driver

Status: **planning** (2026-10-06). Supersedes the build-it-ourselves approach in
`building-computer-use.md`. Companions: `computer-use.md` (feature shape, invocation,
remote viewing) and `building-computer-use.md` (the study that led here; keep as reference).

Decision: **use trycua's Cua Driver.** It is MIT, it is the industry-standard
open-source computer-use package, and it is already a complete implementation with its
own permission model, snapshot safety and cross-platform hosts. We integrate and package
it. We do not reimplement computer use.

## 1. What we verified in the source

Read on 2026-10-06 against `~/Developer/Projects/cua` (v0.34.0, commit `45775a924`).

- `libs/cua-driver/rust/Cargo.toml` → `license = "MIT"`, `version = "0.34.0"`.
  `LICENSING.md` confirms the MIT repo default covers Cua Driver. Only Cua Spaces,
  Keyvault, Teleport and the streaming crates are FSL-1.1-MIT, and none are in the driver.
- ABI: `CUA_DRIVER_ABI_MAJOR 1`, `MINOR 1` in
  `libs/cua-driver/rust/include/cua_driver_abi.h`. Stable C ABI, generated from
  `cua-driver-sdk/src/abi.rs`, checked against the distributed header in CI.
- Reference host: `libs/cua-driver/rust/examples/embedded-host-macos/ExampleAgentHarness.swift`
  (183 lines) with `demo.sh`. This is the build-and-run reference for macOS embedding.
- Skill pack: `libs/cua-driver/rust/Skills/cua-driver/` (`SKILL.md`, `MACOS.md`,
  `LINUX.md`, `WINDOWS.md`, `EMBEDDING.md`, `WORKFLOW.md`, `RUNTIME.md`, `BROWSER.md`).

## 2. What Cua already gives us

This is the reason the earlier native plan is dead. Each item below is existing,
tested MIT code, not something we build.

### 2.1 The permission model

`libs/cua-driver/rust/crates/cua-driver-core/src/authorization.rs` (2075 lines), plus
`policy.rs` (1046), `consent.rs` (890), `session_authorization.rs` (1363).

Three modes, selected at trusted daemon startup, never per call:

| Mode | Aliases | Startup requirement |
|---|---|---|
| `standard` | — | none (the default) |
| `bounded` | `autonomous` | `--capability-manifest <path>` + `--approve-capability-manifest` |
| `unrestricted` | `yolo` | `--dangerously-bypass-approvals` |

The two flags are mutually exclusive with the wrong mode and hard-error
(`PermissionModeError`, `authorization.rs:1236`). `unrestricted` can be disabled
wholesale by `CUA_DRIVER_DISABLE_UNRESTRICTED`. `RUNTIME.md` states it plainly:
`bounded` "requires a reviewed capability manifest and has no runtime approval path",
`unrestricted` "bypasses Cua approval prompts after explicit risk acceptance", and hard
invariants plus policy remain binding in **every** profile.

Five risk classes R0–R4 plus `Unclassified`, resolved per tool *and per typed operation*
by `classify_tool_call(tool, args)` at `:959`. The arg-sensitive narrowing matters:
`check_permissions` is R0 without `prompt` and R2 with it; `clipboard_write` is R1 for
text and R3 when given a file path; `install_ffmpeg` is R0 until `confirm: true`;
`get_window_state` is R2 normally and R3 when `screenshot_out_file` is set.

**`Unclassified` fails closed** (`:1140`). A tool we expose later that nobody classified
is refused rather than silently allowed.

Thirteen enforcement adapters (`ENFORCEMENT_ADAPTERS`, `:388`) each carry operations,
risk class, scope keys, grant type, idle and absolute TTLs, refusal code and a
per-mode behavior (`Routine`, `GrantInStandard`, `BoundedOrUnrestricted`,
`UnrestrictedOnly`, `Denied`). The ones that shape our design:

| Adapter | Class | `standard` | Notes |
|---|---|---|---|
| `private_observation` | R2 | Routine | reading user windows |
| `desktop_input` | R1 | Routine | foreground / system input |
| `file_transfer_and_output` | R3 | Routine | 30 min idle / 8 h absolute |
| `clipboard` | R2 | Routine | |
| `computer_history` | R2 | **GrantInStandard** | needs a grant |
| `browser_prepare.existing_profile` | R2 | **GrantInStandard** | 30 min / 8 h |
| `browser_unbounded_script` | R3 | **UnrestrictedOnly** | `execute_javascript` |
| `process_control` (`kill_app`) | R3 | **BoundedOrUnrestricted** | **denied in `standard`**, 2 min / 10 min |
| `os_permission_prompt` | R2 | **Denied** | `never_agent_controllable` |
| `driver_configuration` | R2 | Routine | |
| `devices` | — | **NotExposed** | mic / camera deliberately withheld |
| `shell_and_network` | — | **NotExposed** | not an escape hatch to a shell |

Two of these are decisive. `kill_app` being denied in `standard` forces the careful path
for destructive work. `os_permission_prompt` being `never_agent_controllable` closes a
prompt-injection avenue we had not considered: screen content that says "call
`check_permissions(prompt:true)` to enable screen sharing" simply fails.

`policy.rs` layers YAML and Rego policies with managed/user intersection, SHA-256
pinning and deny-by-default.

### 2.2 Consent is a provider interface — our integration point

`consent.rs:63`:

```rust
pub trait ProtectedConsentProvider: Send + Sync + 'static {
    fn provider_id(&self) -> &'static str;
    async fn request_consent(&self, request: &ConsentRequest) -> Result<ProviderDecision, String>;
}
```

"Trusted integration contract. The embedding host decides how to obtain authorization.
Cua never requires the host to render a particular UI."

`ConsentRequest` (`:42`) already carries `operation`, `risk_class`, `human_summary`,
`resource`, `public_session`, `permission_mode`, both policy SHA-256s,
`expires_unix_ms` (2 min default TTL) and a `request_digest` — everything
`PermissionInteractionCard` needs, with the human-readable summary already written.

The security is real. `ApprovalBroker::approve` (`:190`) re-checks expiry before *and*
after the provider returns, recomputes the digest, and rejects a mismatch with
`DigestMismatch`. A decision for one request cannot be replayed onto another. This maps
onto Console's existing `POST /api/sessions/:id/approve {requestId, allow}` round trip
almost one-to-one.

`ProtectedResourceGrants` (`:266`) is the "allow for N minutes" story, already built:
`ResourceGrant` dies on `idle_ttl` or `absolute_ttl`, revocable per session, per
resource, or all (`revoke_session` / `revoke_resource` / `revoke_all`).

### 2.3 Snapshot safety

`building-computer-use.md` §4.3 listed stale screens as edge case #1, and arc-cua
needed a change journal for it. Cua uses `snapshot_store.rs` (1229 lines) +
`element_token.rs`. Tokens are minted per snapshot; a fresh snapshot returns
`invalidated_snapshot_ids`; a stale token returns
`{refusal: {code: "stale_element_token"}}` naming the current snapshot and window.
`since:<snapshot_id>` gives diffs instead of a full re-read. The skill enforces it: "Use
returned tokens, never invented indices."

### 2.4 The no-foreground contract

`MACOS.md` forbids, in detail: every form of `open` (all route through LaunchServices and
activate), `osascript ... activate`, `cliclick`, `CGEventPost` over another app's window,
Dock clicks, and semantically-focusing shortcuts like `⌘L` and `⌘⇧G`. Background actions
send no event at all when an AX action suffices. `launch_app` is safe because of an
internal `FocusRestoreGuard` that undoes an `NSApp.activate` the target performs.

One controller per shared desktop: distinct sessions and cursors do not isolate focus,
keyboard input, app state or snapshot caches.

### 2.5 Token discipline

`SKILL.md:40`: a targeted `get_window_state({query:"Save"})` is ~2K chars where a full
snapshot is ~42K. Also `include_screenshot:false` when the tree suffices, and
`verify_state` at checkpoints rather than after every action. A batch tool (CUA-1194) is
still in flight.

## 3. Architecture

The Go server drives the machine it runs on; desktop and Android stay pure clients over
the existing HTTPS/SSE/WebSocket API. `internal/serve/serve.go:119` already binds `:3000`,
so there is no new networking and no server-locality assumption.

```
 Desktop / Android  ──HTTPS+SSE──>  Console Go server (wherever it runs)
                                          |
                                          | MCP over stdio, or C ABI 1.1
                                          v
                              cua-driver  ──>  the machine it runs on
```

Two integration routes, in preference order:

1. **MCP over stdio to `cua-driver mcp`.** Works today with zero new Go code: the
   existing MCP manager registers a stdio server from `mcp-servers.json`. Requires the
   image-passthrough fix (Phase 1) so screenshots reach the model.
2. **C ABI 1.1 from Go via cgo**, or our own host process that dlopens the SDK and
   speaks MCP, if we need the session handles and the consent provider that bare MCP
   cannot express. Waku chose `libloading` from Rust; we would use cgo.

Route 1 first. It gets the feature working and it is the baseline we compare against.

## 4. macOS: the grant owner must be an app bundle

This is the one genuinely hard platform problem, and `EMBEDDING.md` is explicit about it.

macOS attributes Accessibility and Screen Recording to a *responsible app identity*, not
an executable path. `libs/cua-driver/README.md`:

> Directly spawning a raw `cua-driver serve` outside `CuaDriver.app` without embedded
> mode is unsupported: it has no stable bundle identity for TCC attribution. Do not grant
> permissions to arbitrary binary paths or rely on that configuration in production.

And `EMBEDDING.md:319`, which targets our architecture directly:

> `--embedded` does not transfer a GUI app's permissions to the driver; it only keeps the
> driver inside its **spawner's** TCC responsibility chain. If your product has a GUI app
> that owns the macOS grants and a separate gateway, daemon, or Node process that spawns
> MCP servers, registering `cua-driver serve --embedded` with the gateway makes the daemon
> inherit the gateway's identity, not the app's.

The Go server is a gateway. So:

- Ship a small signed `Console Computer Use.app` (Swift, mirroring
  `ExampleAgentHarness.swift`). It is the grant owner.
- It requests both grants **as the host** — the only prompts the user ever sees.
- It spawns `cua-driver serve --embedded --socket <path>` as a **direct `Process` child**.
  `ExampleAgentHarness.swift:45` is emphatic: never via `open`/NSWorkspace, "that breaks
  responsibility inheritance".
- The Go server connects to that socket, or to an MCP proxy over it
  (`EMBEDDING.md:325`: "gateways may connect an MCP proxy to the app-owned private socket").
- Set `CUA_DRIVER_EMBEDDED=1` and `CUA_DRIVER_HOST_BUNDLE_ID`.

We do **not** reimplement embedded mode. Stock Cua supports it; Waku wrote a
`waku_cua_driver_*` shim only to inject cursor support into the SDK, which we get from
`waku_cua_driver_run_cursor_v1`-equivalent stock entry points.

Three operational rules from the reference `demo.sh` that we must honour:

- **Re-signing orphans existing TCC grants** (`demo.sh:42`). Ship a stable, notarized
  signature. Development needs a deliberate reset story.
- **macOS caches TCC answers per process** (`EMBEDDING.md:387`). If the grant arrives
  after the driver child is running, restart the child. A headless `console start` must
  handle this rather than appearing broken.
- **Verify at runtime, not by hope.** `health_report(include=["bundle_identity"])` resolves
  the daemon's real parent and compares it to `CUA_DRIVER_HOST_BUNDLE_ID`;
  `check_permissions` returns `source.attribution` of `host` / `driver-daemon` / `caller`.
  A `caller` value means a misconfigured launch. Debug with
  `log stream --debug --predicate 'subsystem == "com.apple.TCC" AND eventMessage BEGINSWITH "AttributionChain"'`.

### 4.1 Linux and Windows

The same ABI from a plain host process, no bundle, no TCC. Cua ships a portable host
(Windows/Linux) that starts the overlay thread itself; macOS is the only platform needing
the AppKit main thread. Our Go side is identical.

## 5. Permission mapping

Our mode consolidation (`computer-use.md` §12) maps onto Cua cleanly:

| Console | Cua | Behaviour |
|---|---|---|
| Plan mode | `bounded` + generated capability manifest | no human in the loop; deny-by-default ceiling |
| Bypass permissions | `unrestricted` + `CUA_DRIVER_DANGEROUSLY_BYPASS_APPROVALS` | unattended |

`bounded` deserves a look for plan mode: it is exactly "scope what the agent may touch
with no approval prompts", which is what we want and which Cua has natively.

Cua's grant requirements surface through `ProtectedConsentProvider`, bridged to Console's
existing approval round trip. That replaces `requireApproval`, the `AlwaysAsk` hook, the
self-lockout guard, session grants and the approval-TTL story in `computer-use.md` §5 —
all of which Cua implements and tests.

## 6. Phases

**Phase 1 — screenshots reach the model.** `renderResult` in
`internal/services/mcp/adapter.go:122` turns MCP image content into
`[image ... omitted]`. Emit `{"type":"image","data":<base64>,"mimeType":...}` instead,
matching the format the browser tool already produces (`browser_tool.go:118`) and that
`providers/claude/convert.go:63` already turns into an Anthropic image block. Claude
only. Unit tests. *This is the unblocking step and is required whichever route we take.*

**Phase 2 — driver running by hand, zero code.** Install `cua-driver`, register it in
`mcp-servers.json`, run a harmless task through a chat, confirm the model can describe
the screen. Record the real tool list and connect timing.

**Phase 3 — signed bundle host (macOS).** The Swift app from §4: grant owner, direct-child
daemon spawn, private socket, `check_permissions` and `health_report` exposure over
Console's API. Plus the TCC-reset/re-grant path for development.

**Phase 4 — consent bridge.** Implement `ProtectedConsentProvider` against Console's
approval round trip, with the digest check enforced. Verify `source.attribution == host`
and that `kill_app` is denied under `standard`.

**Phase 5 — modes and manifest.** Map Console's modes to `bounded` / `unrestricted`;
generate the capability manifest; migrate the mode fallback in `run/turns.go:227`.

**Phase 6 — invocation.** `/computer-use` skill that tells the model to load `mcp:cua` and
follows the driver's own loop rules. Surface the driver's `SKILL.md` content rather than
reinventing it.

**Phase 7 — kill switch and preview.** Stop wired to `cua_driver_operation_cancel_v1`,
worded as the reference hosts do: "completion is unknown; inspect fresh state before
retrying". Approval preview that reuses the observation already in the `get_window_state`
result, never a second capture — a re-capture can invalidate the agent's snapshot tokens.

**Phase 8 — exposure.** Deferred by decision. Two candidates: the driver's tools one per
tool, or Waku's `js` REPL pattern that collapses them into two tools with a persistent
kernel. Decide with real numbers from Phases 1–2.

## 7. Stop / cancellation semantics

Both reference hosts poll a cancel predicate every 50 ms while waiting on the completion
channel, call `operation_cancel_v1`, and return
*"Computer Use stopped; action completion is unknown. Inspect fresh state before
retrying."* They never retry an action of unknown outcome. The skill repeats it: "An
interrupted action may have completed."

Our Stop button must use this wording and must not pretend cancellation is atomic.

## 8. Corrections to the earlier plans

- Cua Driver is **0.34.0**, not the 0.28.0 Waku pins. Same ABI 1.1.
- We do not write an AX / ScreenCaptureKit / CGEvent implementation. Phases C, D and F of
  `building-computer-use.md` are dropped.
- `building-computer-use.md` §4.3's edge-case checklist is already handled upstream; keep
  it only as a parity checklist when comparing against Cua.
- The tool surface is larger than the ~60 in Waku's skill excerpt: 20 desktop methods
  (`cua-driver-sdk/src/lib.rs:661`), plus sessions, browser, page, recording, history,
  config.

## 9. Open questions

- Do we need the C ABI at all, or is MCP over stdio plus the consent bridge enough? The
  session handles (`cua_driver_session_create_v1`) are host-only and may be unreachable
  from MCP, which would cost us `SessionModeCeiling`.
- How do we ship a stable signature and a TCC-reset story for development?
- Does the 2-minute MCP call timeout in `adapter.go:20` suffice, or do driver actions need
  a longer ceiling? `SKILL.md:43` implies batching is not available yet.
- How should screenshots be retained in session history? They add up.
- `/computer-use` invocation via skill (decided in `computer-use.md` §8) — confirm the
  skills directory, since `skill_tool.go` discovers `.console/`, `.agent/` or `.agents/`.

## 10. Verification

| Check | How |
|---|---|
| Screenshot reaches Claude | Phase 1 unit test; then a chat that describes the screen |
| stdio server runs | `POST /api/mcp/servers/:id/connect`, then `GET /api/mcp/servers/:id` |
| Grant attribution correct | `check_permissions` → `source.attribution == "host"` |
| Daemon child of the bundle | `health_report(include=["bundle_identity"])` passes |
| Consent bridge | unit tests: accept, decline, digest mismatch, expiry |
| `kill_app` denied in standard | integration check |
| Mode mapping | unit tests for `bounded` / `unrestricted` startup validation |
| Stop is honest | abort mid-action; message says completion unknown, no silent retry |