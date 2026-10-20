# Replacing CEF with a native WKWebView driver

CEF is in the desktop app for one reason: `agent-browser` speaks CDP, and CDP is
Chromium-only. The embedded WKWebView host already exists (`SystemWebviewHost`,
used by the Devices tab), but the agent cannot drive it, so the browser tab
stays on Chromium.

A read-only review of `agent-browser` (v0.39.1) showed that its quality comes
from the **snapshot and refs**, not from the transport. Its own Safari/iOS
WebDriver path has no snapshot at all — `handle_snapshot` requires the CDP
browser. That makes the page model the engine-specific core, and it is
portable: the driver is injected JavaScript plus a small amount of native
WKWebView work.

The desktop already executes browser actions locally (`browser_actions.rs`);
the server only dispatches intents through the `browser` tool. This plan
replaces the desktop's CEF implementation and leaves the server-side agent
loop, tool schema, policy, and request/response protocol untouched.

Shape: driver lives in the desktop, in-process, behind the existing
browser-action boundary. The server barely changes.

---

## Module layout

```
apps/desktop/crates/console-ui/src/browser/driver/
  mod.rs            # Rust side: commands, refs state, result parsing
  js/snapshot.js    # tree builder + ref registry (the core)
  js/actions.js     # resolve, hit-test, click, type, select, check
  js/waits.js       # selector/text/url/js/load conditions
  js/content.js     # innerText / outerHTML extraction
  native.rs         # NSEvent injection, dialogs, downloads, popups hooks

apps/desktop/src/state/browser_actions/
  webkit.rs         # NEW: action dispatch for the WebKit backend
  agent_cdp.rs      # existing; compiled only with the `cef-browser` feature
```

The JS files go in as `include_str!`, no build step. They are engine-agnostic
(DOM + standard events), so the same driver can later run on Android WebView.

---

## Phase 0 — Contract and plumbing (1–2 days)

- **Freeze the snapshot grammar** as the shared contract. Keep the current line
  shape, tighten it:

  ```
  - button "Submit" [ref=e3]
  - textbox "Email" (value="a@b.c") [ref=e4]
  - iframe "Checkout"
  ```

  Document it in `proto/console/v1/` as a comment or a short spec; update the
  `browser` tool description so the model and client cannot drift.
- **Fix `run_js` return values.** Today `host.rs` calls `evaluateJavaScript`
  with `completionHandler: nil`. Reuse the existing `agent_script.js` IPC
  result path (request id + `script_result`), or use `callAsyncJavaScript`.
- **Feature flag**: `webkit-browser` / `cef-browser` cargo features. Default
  stays CEF until parity; `browser_actions.rs` picks the implementation at
  compile time.
- **Fixture server + harness**: local pages that exercise the hard cases —
  React-controlled inputs, shadow DOM, cross-origin iframe, overlay covering a
  button, lazy content, dialogs.

---

## Phase 1 — Snapshot + refs (the core, dominant quality)

This is the piece that decides whether the whole thing feels good.

- DOM walk computing implicit roles, accessible names, visibility, and states.
  Reuse and extend `element_script.js`; do not start over.
- Modes: interactive-only, content-only, depth cap, selector scoping.
- Pruning: drop ignored/invisible, merge text runs, collapse empty generics,
  keep sibling groups.
- **Refs**: monotonic `eN` per document, never recycled; `WeakMap<Element, ref>`
  + `Map<ref, descriptor>`; cleared at snapshot start; descriptor (`role`,
  `name`, `nth`) kept for stale fallback. Navigation wipes the JS context, so
  invalidation is automatic.
- Selector scoping includes open shadow roots (`el.shadowRoot`). Closed roots
  are out of reach — accept and document.
- Iframes: same-origin via `contentDocument`; cross-origin frames emit a node
  only. (Per-frame injection is Phase 4.)
- Cap output (~200 lines default) with an explicit truncation line and a hint
  to scope.

**Acceptance** (port their e2e cases): refs never recycle; refs survive DOM
updates; refs invalidate on navigation; selector scoping includes shadow roots;
no duplicate elements.

---

## Phase 2 — Actions (about a week)

- **Resolve**: fast path by ref; fallback by descriptor; stale →
  `Element ref eN is stale` + fresh snapshot; unknown → `Unknown ref`.
- **Hit-test**: `scrollIntoView` → `elementFromPoint` → descend same-origin
  iframes and shadow-including ancestors; if blocked,
  `Element is covered by <div#banner>`.
- **Click**: pointerdown → mousedown → focus → pointerup → mouseup → click.
- **Type**: focus, clear via prototype value setter + `input` event, set text,
  dispatch `input`/`change`, Enter via real key sequence +
  `form.requestSubmit()` fallback.
- **Select**: strict match on value/label/text; dispatch `input`/`change`;
  error lists available options.
- **Check/uncheck**: click, re-read state, label/control fallback.

Error strings matter as much as the actions — the agent recovers from them.

---

## Phase 3 — Observe (3–5 days)

- `wait_for`: selector (visible/attached/detached/hidden), text, URL, JS
  expression; 100 ms poll from Rust; timeout 10s default, 60s max. Load state
  from `WKNavigationDelegate`.
- `get_content`: port the existing `content_script`; add `outerHTML`.
- **Screenshot**: native `takeSnapshot`; add selector + annotated mode (inject
  numbered labels, capture, remove — same trick as theirs). Full-page behind a
  resize hack, or defer.

---

## Phase 4 — Native parity (1–2 weeks)

- `run_js` async + exception text.
- Popups: `createWebViewWithConfiguration` → new Console tab.
- Dialogs: `WKUIDelegate` (auto-accept alert, block actions while a dialog is
  open).
- Downloads: `WKDownloadDelegate`.
- Tabs: one `WKWebView` per tab; already how the host works.
- Cross-origin iframes: per-frame `WKUserScript` + `WKScriptMessage.frameInfo`,
  aggregate snapshots. Verify in the spike; if it is ugly, degrade to "iframe
  present, not inspectable".

---

## Phase 5 — Trusted input gate (2–3 days, then a decision)

- Synthesize `NSEvent` into the key webview; fall back to `CGEvent`.
- Test on a real Google login, Stripe checkout, and a Cloudflare-protected page.
- Outcome: either full parity, or JS-level input with a documented
  "bot-hardened sites may reject" caveat. **This is the only item that can
  change the outcome of the project**, so do it before writing the native
  parity layer.

---

## Phase 6 — Differential harness + flip

Two axes, not one:

1. **Capability**: our driver on WKWebView vs `agent-browser` on CEF. Same
   fixtures, same tasks. Proves we match the thing we like.
2. **Engine**: our driver on WKWebView vs our driver on CEF. Isolates WebKit
   quirks from driver bugs.

Port their e2e scenarios plus ours. Gate the default flip on that corpus being
green — not on vibes.

---

## Phase 7 — Delete Chromium

Remove `browser/cef/`, `agent_browser.rs`, the npm auto-install,
`agent_driver`, the debug port, CEF from `Cargo.toml`, and the CEF bootstrap in
`main.rs`. Update CI and the browser docs.

---

## Scope guard

Explicitly **not** building: implicit auto-wait, auth vault, HAR/trace,
screencast, closed shadow DOM, full `agent-browser` CLI surface. The agent uses
7 verbs — match those well.

---

## Rough sizing

Phases 0–3 (the vertical slice that already feels better than today): ~3–4
weeks. Phase 4: ~1–2 weeks. Phase 5: days plus a decision. Phase 6: ~1 week.
**Flip around week 6–8**, with hardening after.

---

## First deliverable

A vertical slice: WKWebView tab + snapshot + click + type + screenshot, driven
through the real `browser` tool on a local fixture. That is days, not weeks,
and it proves the plumbing before the long tail.
