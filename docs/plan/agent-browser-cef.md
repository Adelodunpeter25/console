# agent-browser on the CEF Browser Plan

Use [agent-browser](https://github.com/vercel-labs/agent-browser) to drive Console's built-in browser, **only when the CEF backend is enabled** (`cef-browser` cargo feature). WKWebView builds keep the current JS-based action path unchanged.

## Status

| Phase | State |
|---|---|
| 1. Foundation (target id per browser, ephemeral debug port) | Done |
| 2. `agent_browser` module (batch builder, discovery, auto-install, runner) | Done |
| 3. Dispatch (`snapshot`, `click`, `type`, `run_js`, `get_content`, `wait_for`, `screenshot`) | Done, needs live end-to-end test with a real agent run |
| 4. Bundling the pinned binary in the app | Not started (auto-install covers dev for now) |
| 5. Hardening (target-gone recovery, concurrency, disconnects) | Not started |

Code map: `console-ui/src/browser/cef/agent_browser.rs` (module), `console-ui/src/browser/agent_driver.rs` (feature-neutral entry point and command builders), `console-ui/src/browser/cef/{client,host,runtime}.rs` (target id, debug port), `apps/desktop/src/state/browser_actions/agent_cdp.rs` (dispatch). Tests: `console-ui/tests/browser/{agent_browser,agent_driver,cef_target_id,cef_debug_port}_test.rs`.

## 1. Why

The `browser` agent tool is limited today because WKWebView has no CDP. Every capability (snapshot, click, type, run_js) is hand-built JS over a `console.log` IPC bridge with 150ms polling (`apps/desktop/src/state/browser_actions.rs`, `element_script.js`, `agent_script.js`). CEF is Chromium, so it can expose CDP, and agent-browser is a native Rust CLI/daemon that attaches to any CDP endpoint and gives us a real accessibility snapshot, real input events, screenshots, network, and more.

Goal: the agent uses the **same visible tabs the user sees**, with proper browser control, and **never steals focus**. Today, when the agent opens a browser tab the webview does not take focus, so if the chat/composer is focused it stays focused. That must hold for every agent-browser action too: opening, navigating, clicking, typing, screenshotting. Nothing else changes for users on the wry backend.

## 2. Verified by spike (agent-browser 0.38.2, CEF 154, macOS)

Run against the live dev app with a fixed debug port and four tabs open (one visible, three hidden):

- CEF accepts `remote_debugging_port` in `cef::Settings`. The debug server only starts after CEF initializes, which is when the first tab navigates (browsers are created lazily).
- `agent-browser --cdp 9333 ...` attaches to the embedded browser. Each Console tab is one CDP page target.
- Works on the visible tab: `snapshot` (a11y tree with `@e` refs), `eval`, `click`, `fill`, `screenshot`.
- Works on **hidden** tabs too (`document.visibilityState` is `hidden`): `eval`, `snapshot`, full (non-blank) `screenshot`, `click` (navigated), `fill` (value set). The app does not have to bring a tab to the front.
- `agent-browser --cdp <port> tab <32-hex-targetId>` pins the session to one exact CDP target, even when two tabs show the same URL. `tab list --json` exposes `targetId`. `--pin-tab` keeps a session bound to its tab and the binding is persisted per session.
- `tN` ids (`t1`, `t2`) are reassigned from `t1` on every new connection and the order changes. They must **never** be used.
- Connecting with a per-page WebSocket URL (`ws://.../devtools/page/<id>`) does **not** pin the target. It still attaches at browser level.

Second spike (four tabs, two of them on the same URL):

- **`Target.getTargetInfo` works per browser, in-process.** `host.add_dev_tools_message_observer(...)` plus `host.execute_dev_tools_method(1, "Target.getTargetInfo", None)` on a freshly created browser returns `{"targetInfo":{"targetId":"<32-hex>", ...}}` immediately, even while the page is still `about:blank`. All four ids matched `/json/list` exactly, and the two same-URL tabs got different ids, so the Console tab to target mapping is reliable. A tab keeps its target id across navigation (a link click on a tab did not change its id).
- **Focus:** with the chat composer focused, `tab <targetId>` (hidden tab), `fill` (hidden tab), `screenshot`, and `click` on another tab did not move focus. The composer cursor stayed put (confirmed by the user) and the app stayed the frontmost application. Not yet tested: a visible tab, `type` with Enter, keyboard commands, and long-running sessions.

Not verified yet: closing a tab through CDP, an ephemeral port, multi-session concurrency, target id after cross-process navigation.

## 3. Hard rules (learned the hard way)

1. **Always pass `--cdp <port>` on every call.** Without it, a session with no live connection silently launches a managed Google Chrome (`--remote-debugging-port=0`). This happened once in the spike. There is no attach-only flag in 0.38.2. (Optional later: patch one in, see 9.)
2. **Never run `agent-browser install`.** Attach mode needs no downloaded browser. (Installing the CLI package itself with npm or bun is fine and is what auto-install does.)
3. **Address tabs by CDP `targetId` only**, never `tN`.
4. **Refs are cleared on every tab switch and navigation** and live in one per-session store. A "switch to tab, then snapshot" must be one atomic step (`batch`), and refs are only valid until the next snapshot or navigation.
5. **Never steal focus.** The agent acting on a tab (visible or hidden) must not move keyboard focus out of the chat/composer or raise the window. CDP input events (`Input.dispatch*`) go straight to the renderer and should not change the AppKit first responder, but some calls can: `Page.bringToFront`, `Target.activateTarget`, `Emulation.setFocusEmulationEnabled`, and any agent-browser command that calls them (including `tab <id>` switching, which may activate the target). **Must be verified** (see 11) and, if any command steals focus, either avoid it or restore focus afterwards (Console already tracks first-responder changes via `ResponderObserver` in `cef/host.rs`).
6. **Never let agent-browser create or close tabs.** `tab new` and `tab close` would create or destroy CDP targets that Console's UI does not know about. Tab lifecycle stays in Console (existing `navigate`, `switch_tab`, `close_tab` handling). agent-browser only operates on page content of an existing target.

## 4. Architecture

The Go server can run on a different machine than the desktop, so the **desktop** runs agent-browser, not the server.

```
Go server (browser tool)
   |  BrowserActionRequest over SSE (protocol unchanged)
   v
Desktop: browser_actions.rs
   |-- backend = wry   -> existing JS path (unchanged)
   '-- backend = CEF   -> agent_browser module
         |  resolves Console tab id -> CDP targetId
         |  spawns: agent-browser --cdp <port> --session <ws> batch "tab <targetId>" "<cmd>"
         v
       agent-browser daemon  --CDP-->  CEF (127.0.0.1:<port>)
```

The server needs no change for the first version. Results flow back through the existing `ResolveBrowserActionDto` path.

### 4.1 Feature gating

Everything new is compiled only for CEF builds:

- Rust: `#[cfg(all(target_os = "macos", feature = "cef-browser"))]` on the new desktop module(s), same gate as `browser::cef`.
- The runtime choice is per tab/backend. In a CEF build, CEF tabs go through agent-browser; if the CDP port or the `agent-browser` binary is unavailable at runtime, **fall back to the existing JS path** (log once, do not fail the action).
- WKWebView builds never reference agent-browser and do not ship the binary.

## 5. Components

### 5.1 CEF debug port (desktop, CEF side)

- Done (spike): opt-in `CONSOLE_CEF_DEBUG_PORT` env var sets `remote_debugging_port` (1024-65535, otherwise off). See `crates/console-ui/src/browser/cef/runtime.rs`.
- **Done:** always on in CEF builds, on an ephemeral loopback port. Instead of an `App` handler plus reading `DevToolsActivePort`, the app binds `127.0.0.1:0`, reads the free port, releases it and passes it as `Settings.remote_debugging_port` (`runtime::resolve_debug_port`). `CONSOLE_CEF_DEBUG_PORT` still pins a port. `runtime::debug_port()` returns it once CEF is initialized. There is a tiny window between releasing the port and CEF binding it; acceptable for a loopback dev endpoint.
- The port only exists after CEF initializes (lazily, at the first navigating tab). `agent_driver::is_ready()` is false until then, and callers use the in-page fallback.

### 5.2 Tab to target mapping

Each Console tab (`browser_id` in `browser_actions.rs`, resolved by `pick_browser_view`) must map to exactly one CDP `targetId`.

- **Done (`cef/client.rs`, `cef/host.rs`): `WebviewHost::target_id()` returns the id and the observer registration lives as long as the host.** Verified on CEF 154: ask each CEF browser for its own target id with a DevTools method on its host (`Target.getTargetInfo` via `execute_dev_tools_method`, result delivered to a `DevToolsMessageObserver.on_dev_tools_method_result`) right after the browser is created, and store it on the `WebviewHost`/`BrowserView`. The call must run on the CEF UI thread (we are on the main thread there). Keep the observer `Registration` alive for the host's lifetime and drop it with the host. The throwaway spike code that logs this lives in `cef/client.rs` (`TargetInfoObserver`) and `cef/host.rs`; productionize it by replacing the logging with storing the id.
- Fallback if that does not work: list targets over `http://127.0.0.1:<port>/json/list` and match by URL+title, resolving ambiguity (duplicate URLs) by temporarily tagging the page. This is fragile; avoid unless needed.
- Targets get destroyed and recreated on cross-process navigation in some cases. Re-resolve the target id when an action fails with "target gone", and refresh on browser create/close events.

### 5.3 `agent_browser` module (new, desktop)

Responsibilities:

- Locate the binary: bundled in the app (`Contents/Resources/` or next to the main binary) first, then `PATH` in dev. Pin the version like `gpui-component`.
- **Done.** One blocking `std::process` call per request with a timeout (kills a hung process), run on gpui's background executor. Output is `--json`: an array of `{command, error, result}`.
- Argv is always `agent-browser --cdp <port> --session <session> --json batch --bail`, and the batch is written **on stdin as JSON**: `[["tab","<targetId>"],["<verb>", ...args]]`. Verified: args are JSON strings, so agent-supplied text that looks like flags (`--session evil`) stays data and nothing needs shell quoting. The leading `tab` step makes the switch and the action atomic; if it fails the target is gone (`RunError::TargetGone`).
- Verb allow-list (`snapshot click dblclick fill type press hover focus scroll select check uncheck eval get wait screenshot find is`). `tab`, `close`, `install`, `open`, `connect`, `launch`, ... are refused before anything is spawned. Session names are `[A-Za-z0-9_-]{1,64}`, target ids 32 hex chars.
- Session name: `agent_driver::session_name()` is `console` (or `console-<hash>` when `CONSOLE_CEF_DATA_DIR` is set, so a test build stays separate). One stable name means repeated app runs reuse a single daemon instead of leaking one per run. Per-workspace sessions are still an open question.
- **Binary discovery and auto-install:** `CONSOLE_AGENT_BROWSER_BIN` override, then a copy next to the app (`agent-browser`, `../Resources/agent-browser`), then `PATH`, well-known dirs (`~/.bun/bin`, `~/.local/bin`, `~/.npm-global/bin`, `~/.cargo/bin`, `/opt/homebrew/bin`, `/usr/local/bin`), then the user's login shell (`$SHELL -ilc 'command -v ...'`), because apps launched from Finder do not inherit the shell `PATH` that bun, mise and nvm set up. If not found, `ensure_binary()` installs the pinned `agent-browser@0.38.2` globally with **npm if available, otherwise bun** (falling back to bun if npm fails), one installer at a time. If neither exists or both fail, agent-browser is disabled for the session and actions use the in-page fallback. The first action after a fresh install blocks while it installs.

### 5.4 Dispatch changes in `browser_actions.rs`

- At the top of each action that needs page access (`snapshot`, `click`, `type`, `run_js`, `get_content`, `wait_for`, `screenshot`): if the picked view is a CEF browser with a live host and a known target id, take the agent-browser path. Otherwise the existing path.
- Make sure the browser exists and is rendering before acting (lazy creation: a tab that never navigated has no CEF browser and therefore no target). Navigate/create first, as `navigate` already does.
- `tabs`, `switch_tab`, `close_tab`, `navigate` stay as Console implementations. They keep updating `agent_browser_tab` as today.
- **Done** in `browser_actions/agent_cdp.rs`, hooked into the four handler groups right after the tab is picked. `try_agent_browser` takes the request only when `agent_driver::is_ready()` and the view has a `cdp_target_id()`; otherwise (including any WKWebView build) the old path runs untouched. If agent-browser turns out to be unavailable mid-request (install failed), the same request is re-dispatched to the in-page path.

### 5.5 Action mapping (first version keeps the current tool schema)

| Console action | agent-browser |
|---|---|
| `snapshot` | `snapshot -i` (refs `@eN`; keep the server's `eN` format by stripping `@`) |
| `click` (ref) | `click @eN` |
| `type` (ref, text, submit) | `fill @eN "<text>"`, then `press Enter` if `submit` |
| `run_js` | `eval "<script>"` |
| `get_content` | `get text <selector>` / `snapshot` or `eval` as needed |
| `wait_for` | `wait ...` (selector / text / url) |
| `screenshot` | `screenshot <path>` then attach the image as today |
| `navigate` | Console `load_url` (unchanged), then optional `wait` |
| `tabs` / `switch_tab` / `close_tab` | Console implementation (unchanged) |

Later (optional, needs a tool schema change in `browser_tool.go`): `batch`, `press`/`keyboard`, `hover`, `scroll`, `select`, network/cookies/storage, a11y diff.

## 6. Ref lifetime rules for the agent

- A ref is valid only for the tab it was snapshotted on, and only until the next snapshot or navigation in that tab.
- The tool result for `snapshot` should say which tab it belongs to (already includes `Tab: <id>`).
- On `Unknown ref` / `Could not locate element`, return a clear "take a new snapshot" message instead of retrying silently.

## 7. Security and scope notes

- The debug port is on 127.0.0.1 only. Any local process could use it while the app runs. Acceptable for now (decision: not a focus), but keep it bound to loopback and do not forward it anywhere.
- agent-browser runs with the user's own CEF profile and logged-in sessions, which is the point (same visible tabs). Do not point it at a different browser.

## 7b. Known gaps

- **Target gone:** a failed `tab <targetId>` returns a clear error, but the stored target id is not refreshed (it is read once at browser creation). Re-resolve it on that error (phase 5).
- **Install blocks the first action** and is not subject to the per-call timeout. The server-side tool timeout is unknown.
- **No live end-to-end run yet** with a real agent session; unit tests cover builders, parsing, runner, installer ordering and the driver.
- **Screenshot** writes a temp PNG that is read and deleted immediately.
- **Snapshot format:** `snapshot` is now agent-browser's interactive tree (`snapshot -i`), whose format differs from the old JS snapshot (refs are still `eN`).

## 8. Phases

1. **Foundation** - ephemeral debug port + accessor (5.1). Store each browser's `targetId` from `Target.getTargetInfo` (5.2, verified). Verify tab close via CDP vs Console close (`do_close`).
2. **Module** - `agent_browser` module with binary lookup, argv builder with the hard rules (section 3), timeouts, error mapping, allow-list (5.3).
3. **Dispatch** - wire `snapshot` / `click` / `type` / `run_js` / `screenshot` behind the CEF gate with fallback to the JS path (5.4, 5.5). Then `get_content` and `wait_for`.
4. **Bundling** - ship a pinned `agent-browser` binary in the CEF app bundle (`scripts/build.sh --cef`), including release signing/notarization. Do not ship it in non-CEF builds.
5. **Hardening** - multi-tab and hidden-tab behavior, target-gone recovery, concurrency (sessions per workspace), crash/disconnect behavior (the app quitting mid-command).
6. **Optional** - widen the tool schema (batch, keyboard, network), and the optional attach-only patch (section 9).

## 9. Optional: local agent-browser clone

A read-only clone lives at `~/Developer/Projects/agent-browser` (v0.38.2). It is not required, since stock 0.38.2 covers the integration. Possible small patch if we want a safety net:

- `--attach-only` (and `AGENT_BROWSER_ATTACH_ONLY`): refuse the implicit local launch in `cli/src/native/actions.rs` (`auto_launch_inner`, before the local `BrowserManager::launch` near the end, and the local-launch branch of `handle_launch_inner`). Must also be sent as a per-command field (like `pinTab`), because the daemon ignores env changes after it starts. Follow the clone's `AGENTS.md` (help text, README, skill docs, MCP parity, tests).

## 10. Testing (per repo `AGENTS.md`)

- Rust tests live in dedicated `tests/` files, never inline `#[cfg(test)]` modules. Run only the relevant file, e.g. `cd apps/desktop && cargo test -p <crate> <test_name>`.
- Unit-test the argv builder and the allow-list (always `--cdp`, always `tab <targetId>`, never `tab new`/`close`/`install`).
- Live check (manual, documented): launch the CEF dev app, open 2 to 3 tabs including a duplicate URL and a hidden tab, run snapshot/click/fill/screenshot against each by target id, confirm the visible UI follows.
- Verify build with `cd apps/desktop && cargo check -p console-ui --features cef-browser`, and that the non-CEF build is unaffected.

## 11. Open questions

- Does the CDP close path (`Target.closeTarget`) interact badly with the `do_close` handling? Expect Console to keep owning closes, so this may not matter.
- Where does the binary live in the signed/notarized bundle, and does the hardened runtime need entitlements for spawning it?
- **Focus (mostly answered):** `tab <targetId>`, `fill`, `screenshot`, `click` did not steal focus on hidden tabs. Still to test: a visible tab, `type` plus Enter, `press`/keyboard commands, `hover`/`scroll`, and a stricter check than "the cursor stayed" (e.g. log first-responder changes through `ResponderObserver` while actions run). If any command steals focus, find which CDP call does it and avoid or undo it.
- Do we want one agent-browser session per workspace or per tab? Per workspace is simpler; per tab avoids sharing the single "current tab" state under concurrent actions.
- Dev workflow: how do developers set the port/binary when running unbundled?

## 12. Spike commands (reference)

```bash
# run the CEF dev app with a fixed debug port, open a tab, load a page
cd apps/desktop && CONSOLE_CEF_DEBUG_PORT=9333 ./scripts/dev.sh --cef

curl -s http://127.0.0.1:9333/json/list            # targets (one per tab)
agent-browser --cdp 9333 tab list --json            # includes targetId
agent-browser --cdp 9333 tab <32-hex-targetId>      # pin to one exact tab
agent-browser --cdp 9333 snapshot -i
agent-browser --cdp 9333 click @e1
agent-browser --cdp 9333 screenshot /tmp/shot.png
```
