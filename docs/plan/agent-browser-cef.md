# agent-browser on the CEF Browser Plan

Use [agent-browser](https://github.com/vercel-labs/agent-browser) to drive Console's built-in browser, **only when the CEF backend is enabled** (`cef-browser` cargo feature). WKWebView builds keep the current JS-based action path unchanged.

## 1. Why

The `browser` agent tool is limited today because WKWebView has no CDP. Every capability (snapshot, click, type, run_js) is hand-built JS over a `console.log` IPC bridge with 150ms polling (`apps/desktop/src/state/browser_actions.rs`, `element_script.js`, `agent_script.js`). CEF is Chromium, so it can expose CDP, and agent-browser is a native Rust CLI/daemon that attaches to any CDP endpoint and gives us a real accessibility snapshot, real input events, screenshots, network, and more.

Goal: the agent uses the **same visible tabs the user sees**, with proper browser control, and nothing else changes for users on the wry backend.

## 2. Verified by spike (agent-browser 0.38.2, CEF 154, macOS)

Run against the live dev app with a fixed debug port and four tabs open (one visible, three hidden):

- CEF accepts `remote_debugging_port` in `cef::Settings`. The debug server only starts after CEF initializes, which is when the first tab navigates (browsers are created lazily).
- `agent-browser --cdp 9333 ...` attaches to the embedded browser. Each Console tab is one CDP page target.
- Works on the visible tab: `snapshot` (a11y tree with `@e` refs), `eval`, `click`, `fill`, `screenshot`.
- Works on **hidden** tabs too (`document.visibilityState` is `hidden`): `eval`, `snapshot`, full (non-blank) `screenshot`, `click` (navigated), `fill` (value set). The app does not have to bring a tab to the front.
- `agent-browser --cdp <port> tab <32-hex-targetId>` pins the session to one exact CDP target, even when two tabs show the same URL. `tab list --json` exposes `targetId`. `--pin-tab` keeps a session bound to its tab and the binding is persisted per session.
- `tN` ids (`t1`, `t2`) are reassigned from `t1` on every new connection and the order changes. They must **never** be used.
- Connecting with a per-page WebSocket URL (`ws://.../devtools/page/<id>`) does **not** pin the target. It still attaches at browser level.

Not verified yet: `Target.getTargetInfo` per browser from inside the app (see 5.2), closing a tab through CDP, an ephemeral port, multi-session concurrency.

## 3. Hard rules (learned the hard way)

1. **Always pass `--cdp <port>` on every call.** Without it, a session with no live connection silently launches a managed Google Chrome (`--remote-debugging-port=0`). This happened once in the spike. There is no attach-only flag in 0.38.2. (Optional later: patch one in, see 9.)
2. **Never run `agent-browser install`.** Attach mode needs no downloaded browser.
3. **Address tabs by CDP `targetId` only**, never `tN`.
4. **Refs are cleared on every tab switch and navigation** and live in one per-session store. A "switch to tab, then snapshot" must be one atomic step (`batch`), and refs are only valid until the next snapshot or navigation.
5. **Never let agent-browser create or close tabs.** `tab new` and `tab close` would create or destroy CDP targets that Console's UI does not know about. Tab lifecycle stays in Console (existing `navigate`, `switch_tab`, `close_tab` handling). agent-browser only operates on page content of an existing target.

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
- Target state: ephemeral port, always on in CEF builds, bound to 127.0.0.1 (CEF default).
  - Needs an `App` handler (we pass `None` today) to append `--remote-debugging-port=0` in `on_before_command_line_processing`, then read the bound port from `DevToolsActivePort` in the CEF cache dir. A fixed port collides when a dev build and a test build run side by side (`CONSOLE_CEF_DATA_DIR` is already used for that).
  - Expose the resolved port through a small runtime accessor (e.g. `cef::runtime::debug_port() -> Option<u16>`).
- The port only exists after CEF initializes. The module must treat "no port yet" as "CEF not ready" and trigger lazy browser creation first (see 5.4).

### 5.2 Tab to target mapping

Each Console tab (`browser_id` in `browser_actions.rs`, resolved by `pick_browser_view`) must map to exactly one CDP `targetId`.

- Preferred: ask each CEF browser for its own target id with a DevTools method on its host (`Target.getTargetInfo` via `execute_dev_tools_method`) once the browser is created, and store it on the `WebviewHost`/`BrowserView`. **Must be verified** against CEF 154 before building on it.
- Fallback if that does not work: list targets over `http://127.0.0.1:<port>/json/list` and match by URL+title, resolving ambiguity (duplicate URLs) by temporarily tagging the page. This is fragile; avoid unless needed.
- Targets get destroyed and recreated on cross-process navigation in some cases. Re-resolve the target id when an action fails with "target gone", and refresh on browser create/close events.

### 5.3 `agent_browser` module (new, desktop)

Responsibilities:

- Locate the binary: bundled in the app (`Contents/Resources/` or next to the main binary) first, then `PATH` in dev. Pin the version like `gpui-component`.
- Run one command per request via `tokio::process::Command` (or the existing async runtime) with a timeout, parse `--json` output where available, and map errors to the existing `[tab id] ...` error text.
- Always build the argv as: `agent-browser --cdp <port> --session <session> <command...>`. One session per Console workspace (so refs and the daemon's current-tab state are not shared across workspaces).
- Always run page commands as a `batch` that starts with `tab <targetId>` so the tab switch and the command are atomic: `batch "tab <targetId>" "snapshot -i"`.
- Do not run `tab new`, `tab close`, `close`, or `install`. Maintain an allow-list of subcommands.

### 5.4 Dispatch changes in `browser_actions.rs`

- At the top of each action that needs page access (`snapshot`, `click`, `type`, `run_js`, `get_content`, `wait_for`, `screenshot`): if the picked view is a CEF browser with a live host and a known target id, take the agent-browser path. Otherwise the existing path.
- Make sure the browser exists and is rendering before acting (lazy creation: a tab that never navigated has no CEF browser and therefore no target). Navigate/create first, as `navigate` already does.
- `tabs`, `switch_tab`, `close_tab`, `navigate` stay as Console implementations. They keep updating `agent_browser_tab` as today.

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

## 8. Phases

1. **Foundation** - ephemeral debug port + accessor (5.1). Verify `Target.getTargetInfo` per browser and store the target id (5.2). Verify tab close via CDP vs Console close (`do_close`).
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

- Does `Target.getTargetInfo` work per browser via the host in CEF 154 (5.2)?
- Does the CDP close path (`Target.closeTarget`) interact badly with the `do_close` handling? Expect Console to keep owning closes, so this may not matter.
- Where does the binary live in the signed/notarized bundle, and does the hardened runtime need entitlements for spawning it?
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
