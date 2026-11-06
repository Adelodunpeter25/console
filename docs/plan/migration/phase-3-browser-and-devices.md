# Phase 3 — built-in browser and device streaming

**Goal**: the two webview-backed features, which NSH supports natively.

## Built-in browser tab

- `<WebView>` element (WKWebView / Android WebView / WebKitGTK / WebView2) with `src`, nav-state
  events, `injectJavaScript`, two-way `postMessage`.
- The `window.ipc` bridge the agent flows use is an `injectedJavaScript` shim posting through
  `window.nsh.postMessage` — the page scripts don't change.
- Host gating: `capabilities.webview.hosts` in `nsh.json` restricts what the browser may load.
- Chrome import: `console-core`'s `ChromeService` keeps working as-is; cookie injection into
  `WKHTTPCookieStore` becomes a small native component in the NSH crate (the decrypted values
  still never touch disk).
- Known gap: NSH's WebView has **no screenshot/snapshot API** — needed by browser-use. Either a
  native component wraps `takeSnapshot` per platform, or it's contributed upstream.

## Device streaming (Android emulators / iOS simulators)

- Nothing changes on the server: `sim-go` + the H.264-over-WebSocket player page
  (`internal/routes/device_player.go`) stay as they are.
- The Devices tab embeds `<WebView src={server}/api/devices/player">` with the same ipc shim;
  touch/key input flows back over the existing socket. Proven on Android already; webviews on
  desktop need WebKitGTK/WebView2 recency for WebCodecs.

## Done when

- Browser tab: navigate, agent JS control, annotations, cookie import — on macOS at parity with
  the GPUI app, and working on Windows/Linux for the first time.
- Devices tab streams and controls an emulator/simulator from the NSH desktop app.
