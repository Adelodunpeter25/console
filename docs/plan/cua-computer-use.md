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

### 2.2 Consent exists but we are not using it

`consent.rs:63` defines `ProtectedConsentProvider`, and `cua-driver-sdk/src/lib.rs:63`
exposes `DriverAuthorizationHost` for embedding hosts. `ConsentRequest` carries
`operation`, `risk_class`, `human_summary`, `resource` and a `request_digest`, and
`ApprovalBroker::approve` (`:190`) enforces digest and expiry. It is a complete approval
stack.

**We are deliberately not building it.** Computer use runs `unrestricted`, which needs no
callback, so there is nothing to bridge. This drops the whole approval half of
`computer-use.md` §5: no `requireApproval`, no `AlwaysAsk` hook, no consent bridge, no
approval preview route, no session grants, no self-lockout guard. Cua still enforces its
risk model underneath — we just never ask it for a grant.

The consequence, accepted knowingly: with `unrestricted` and no human in the loop, screen
content can steer the agent directly. `computer-use.md` §12 already accepts this for the
plan/bypass end state.

Note for the record: the `DriverAuthorizationHost` trait is **only** reachable through the
SDK. Every CLI path hardcodes `authorization_host: None` (`main.rs:306`, `serve.rs:2658`,
`sdk_adapter.rs:488`, `private_worker.rs:127`, `mcp_envelope.rs:609`), so an MCP client
cannot supply one at all. Over MCP the only approval mechanism is `--grant <name>` launch
grants. This is one of the reasons we chose the C ABI (below).

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
                                          | dlopen, C ABI 1.1 (cgo)
                                          v
                            libcua_driver_sdk  ──>  the machine it runs on
```

**We dlopen the SDK directly. No MCP layer.**

Waku does the same: `crates/waku-computer-use/src/sdk.rs` is 269 lines of `libloading`
FFI bindings against the same ABI, and it is the only route that gives us the real API.

Reasons:

- **MCP is lossy.** `mcp.ToolName` sanitizes and truncates to 64 chars, and
  `renderResult` flattens content. Direct we get the exact `ToolResult`:
  `content`, `structuredContent`, `isError`, `screenshot_frame_valid`, and native `u64`
  window ids (which overflow 32-bit and must never be truncated).
- **The session handles are host-only.** `cua_driver_session_create_v1` is "a host API,
  not an agent tool", and its handle "cannot be reconstructed from a public session ID".
  Over MCP we would lose `SessionModeCeiling` and per-session authority entirely.
- **It is less code than the alternative.** ~270 lines of bindings, once.

The MCP image-passthrough fix already landed (`renderResult` in
`internal/services/mcp/adapter.go`) is still correct and still benefits other MCP servers,
but it is no longer on the computer-use critical path.

## 4. macOS: the grant owner is a bundle, and a bundle is just a directory

macOS attributes Accessibility and Screen Recording to a *responsible app identity*, not an
executable path. `libs/cua-driver/README.md`:

> Directly spawning a raw `cua-driver serve` outside `CuaDriver.app` without embedded
> mode is unsupported: it has no stable bundle identity for TCC attribution. Do not grant
> permissions to arbitrary binary paths or rely on that configuration in production.

**A bundle is not a language or a package. It is a directory with a plist in it.** Cua's
own bundle demonstrates this — `_install-rust.sh:157` calls it "the `.app` bundle that
wraps the bare binary so the TCC auto-relaunch path has a stable bundle id", and
`_install-local-rust.sh:389` copies the same Rust binary into `Contents/MacOS/`. Identical
file, one copy bare and one copy bundled.

```
Console Computer Use.app/
  Contents/
    Info.plist          <- CFBundleIdentifier + the two usage descriptions
    MacOS/
      console           <- the Go server binary, unchanged
```

So the language inside is irrelevant; TCC keys on the bundle identifier. Our Go server
binary goes in there as-is. Building it is `mkdir`, write a plist, `codesign`. No Xcode
project and no Swift.

Lifecycle: the server currently starts with `console start`, which re-execs with `setsid`
as a bare background process with no bundle identity. When computer use is enabled it must
instead be launched as
`open Console Computer Use.app/Contents/MacOS/console`, so the process inherits the bundle
identity. Two consequences to handle:

- The bundle ships as a directory, not a single binary. Install to `/Applications` or
  alongside the binary; macOS may quarantine a freshly-written bundle on first launch.
- Development signing is ad-hoc (`codesign --force --sign -`), which is fine locally.
  Distribution needs a stable Developer ID, because **re-signing orphans existing TCC
  grants** (`demo.sh:42`). Once real grants exist, a reset story is needed for dev.

Verify rather than assume: `check_permissions` returns
`source.attribution` of `host` / `driver-daemon` / `caller`, and
`health_report(include=["bundle_identity"])` compares the observed parent bundle id against
`CUA_DRIVER_HOST_BUNDLE_ID`. Debug with
`log stream --debug --predicate 'subsystem == "com.apple.TCC" AND eventMessage BEGINSWITH "AttributionChain"'`.

### 4.1 The agent cursor overlay: not shipping it

The macOS overlay is a synthetic second pointer showing where the agent is acting, so you
can watch it work while your real cursor stays put. It needs AppKit on the main thread:
`platform-macos/src/cursor/overlay.rs:486` — `run_on_main_thread()` "must be the OS main
thread... Never returns normally". It is not exported through the public C ABI, which is
why Waku patched its own `waku_cua_driver_run_cursor_v1()` shim into the SDK.

Without a host-owned AppKit loop, exactly four tools return `facility_unavailable`
(`platform-macos/src/tools/mod.rs:748`, asserted in `cua-driver-sdk/src/lib.rs:3293`):
`move_cursor`, `set_agent_cursor_enabled`, `set_agent_cursor_motion`,
`set_agent_cursor_theme`.

**Decision: not shipping it.** Everything else still works — window and app observation,
accessibility trees, background click and type without focus steal, per-window
screenshots, keyboard input, menus, drag, scroll, launch. Only the *visibility* of the
agent's pointer is lost; you watch via screenshots or VNC instead.

This is what removes the Swift requirement. With no AppKit loop to own, the Go server can
own the bundle itself.

### 4.1 Linux and Windows

The same ABI from a plain host process, no bundle, no TCC. Cua ships a portable host
(Windows/Linux) that starts the overlay thread itself; macOS is the only platform needing
the AppKit main thread. Our Go side is identical.

## 5. Permission model: `unrestricted`, full stop

Decision (2026-10-06): computer use runs with full permission. The agent drives the machine
it runs on with no human in the loop.

At C ABI creation this means `cua_driver_create_v1` with
`claude_code_compatibility: false` and startup configuration selecting `unrestricted`
(`CUA_DRIVER_PERMISSION_MODE=unrestricted` plus
`CUA_DRIVER_DANGEROUSLY_BYPASS_APPROVALS=1`, which `authorization.rs:1264` requires
together or it hard-errors).

What still applies underneath, and costs nothing:

- **Hard invariants.** Including the self-lockout we would otherwise have built ourselves:
  the driver refuses any tool call whose `pid` is its own process id
  (`authorization.rs:1178`), and refuses `kill_app` in `standard`.
- **Policy layers.** YAML/Rego managed and user policy remain binding in every profile
  (`policy.rs`), deny-by-default.
- **`Unclassified` fails closed.** A tool Cua has not reviewed is refused, not allowed.
- **`os_permission_prompt` is `never_agent_controllable`.** Screen content cannot talk the
  agent into raising a TCC prompt.
- **`shell_and_network` is `NotExposed`.** Computer use is not a shell escape hatch.
- **Adapters still gate by mode.** Under `unrestricted`, `browser_unbounded_script`
  becomes available; `Routine` adapters keep working.

So Cua's risk model still shapes what is possible, we are just not using it to interrupt.
`bounded` + a generated capability manifest remains the obvious later option if we ever
want a scoped, unattended mode for plan mode (`computer-use.md` §12) — it needs no human
in the loop either, which is exactly the property we want.

Tiering for Console's own approval system (`ResolveTier`) should read Cua's
`risk_metadata_json` rather than infer from tool names. `kill_app` is not `exec` by name; it
is R3 with a bespoke rule.

## 6. Phases

**Phase 1 — MCP images reach the model. DONE.** `renderResult` in
`internal/services/mcp/adapter.go` no longer drops MCP image content; it emits native image
parts matching the browser tool's shape, capped at 4 per result. Correct on its own merits
for any MCP server that returns images, but **no longer on the computer-use path** — we
bypass MCP.

**Phase 2 — dlopen bindings (cgo). DONE.** `internal/services/cua/driver.go` loads
`libcua_driver_sdk.{dylib,so}` and binds all 12 ABI 1.1 entry points. A library missing
even one symbol is rejected rather than half-used. Only an absolute `CUA_DRIVER_LIB_PATH`
or a file beside our own binary is considered; a relative override is refused outright.
Modelled on Waku's 269-line `sdk.rs` and on `internal/fff`, which already does runtime
dlopen in this repo.

**Phase 3 — driver working against the Go server. DONE.** Verified against a real
`libcua_driver_sdk.dylib` built from `libs/cua-driver/rust`:

- ABI 1.1.0, driver 0.34.0, contract 0.8.0, MCP protocol 2025-06-18
- **56 tools advertised**, every one carrying a risk classification
  (r0: 9, r1: 20, r2: 17, r3: 10 — no r4, nothing unclassified)
- `get_screen_size` returns `Main display: 1440x900 points @ 2x`
- `list_apps` enumerates 96 apps, 8 running
- `get_desktop_state` returns a real 2880x1800 PNG, decoded to raw bytes and converted
  to a native image part with `mimeType: image/png`
- Stop refuses a subsequent call with a *known* outcome; Resume restores service

**Phase 4 — the bundle (macOS). NEXT.** `Console Computer Use.app` containing the Go
server binary plus an `Info.plist` with the bundle id and both usage descriptions;
codesign; launch through `open` so the process inherits the bundle identity. Handle the
TCC-reset path for development and expose `check_permissions` over Console's API so the
desktop can show the user what is missing.

Confirmed necessary by the probe: `health_report` reports
`❌ bundle_identity: Process has no CFBundleIdentifier`, and `check_permissions` reports
Accessibility and Screen Recording **not granted**. Metadata and observation calls work
without a grant, but reliable capture and input do not.

**Phase 5 — tool surface. DONE.** `internal/services/cua/{manager,tools}.go` turn the
inventory into harness tools (`cua__<name>`), keeping Cua's own input schema so the model
sees the real argument names. Tiering reads Cua's `risk` object and its `readOnlyHint`
rather than name prefixes — verified on real data: `kill_app` is r3/exec despite having
no exec-ish prefix, and `get_window_state` is r3 but reads as a read, so observation does
not prompt on every screenshot.

**Phase 5a — subagents inherit the parent's mode. DONE.** `subagent.go` took a
hard-coded `permissions.FullAccess` while inheriting the parent's tools, so a subagent
could act with more authority than the run that spawned it. `SubagentContext` now carries
`ApprovalMode`, `run/turns.go` passes the run's mode, and an unset mode falls back to
`always-ask` rather than full-access. Prerequisite for unattended operation.

**Phase 6 — `/computer-use` invocation.** Decided: the agent has no knowledge of computer
use until the command is run. The skill loads the Cua tool group and carries the driver's
own loop rules; prefer Cua's `SKILL.md` content over reinventing it. Note `skill_tool.go`
discovers skills from `.console/`, `.agent/` or `.agents/` directories in the project tree
or home — not a repo-root `skills/`.

**Phase 7 — kill switch.** Stop wired to `cua_driver_operation_cancel_v1`, worded as the
reference hosts do (§7). No approval preview needed — there are no approvals.

**Phase 8 — composer visibility.** Deferred by decision. Seeing where the agent is acting
inside the Console composer, rather than on the desktop, is a later idea and is not the
Cua cursor overlay.

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
- **We do not use MCP.** Waku's agent-facing surface is a C ABI host
  (`crates/waku-computer-use`), and the MCP in `WakuComputerUse.swift` is only internal
  plumbing between their JS REPL and the driver.
- **No approvals.** Full permission by decision, so `computer-use.md` §5's approval design
  is superseded: no `requireApproval`, no `AlwaysAsk` hook, no consent bridge, no preview
  route, no grants UI.
- **No cursor overlay.** Not shipping it, which removes the AppKit requirement and with it
  the need for Swift anywhere.
- **A bundle is not a language.** It is a directory with an `Info.plist`; the Go server
  binary goes inside it as-is. Cua's own `CuaDriver.app` wraps the same Rust binary that
  also exists bare on disk.

## 9. Open questions

- How do we vendor `libcua_driver_sdk` per platform, and how does the server find it? It
  must be an absolute packaged path next to the binary, never `PATH`.
- How do we ship a stable signature and a TCC-reset story for development?
- Does the driver's own call timeout suffice, or do actions need a longer ceiling than
  anything we impose? `SKILL.md:43` implies batching is not available yet.
- How should screenshots be retained in session history? They add up.
- `/computer-use` invocation via skill (decided in `computer-use.md` §8) — confirm the
  skills directory, since `skill_tool.go` discovers `.console/`, `.agent/` or `.agents/`.
- Does `cua-driver` still ship a prebuilt SDK dylib, or must we build it from the Rust
  workspace? `_install-rust.sh` installs a bundle around a binary; the `.so`/`.dylib` we
  dlopen may come from a different artifact.

## 10. Verification

| Check | How |
|---|---|
| Bindings load | unit test that a missing/garbage library fails cleanly, no path search |
| ABI version check | unit test rejecting a mismatched major/minor |
| Driver reaches the machine | **PASS** — real dylib, `get_screen_size` and `list_apps` return live data |
| Screenshot decodes to an image part | **PASS** — real 2880x1800 PNG, `mimeType: image/png` |
| Tool inventory is fully classified | **PASS** — 56/56 tools carry a risk class |
| Tiering matches Cua's assessment | **PASS** — `kill_app` exec, observations read |
| Stop is honest | **PASS** — refused start reports a known outcome; Resume restores service |
| Screenshot reaches Claude | a chat that describes what is on screen |
| Grant attribution correct | `check_permissions` → `source.attribution == "host"` |
| Bundle identity | `health_report(include=["bundle_identity"])` matches our bundle id |
| Tiering | unit test that `kill_app` is not classified by name prefix |
| Stop is honest | abort mid-action; message says completion unknown, no silent retry |
| Invocation is gated | without `/computer-use`, the model has no computer-use tool |