# NSH (Native Shell) migration assessment

Researched 2026-11-06 against https://nsh.dev/docs and the `longbridge/nsh` source at `main`.
Decision: migrating Console's desktop app to NSH — Rust core, **all UI in TSX**. This doc records
what was verified so the migration can start with the findings in hand.

## Why

- Raw GPUI UI code is verbose to style; compile times hurt the UI iteration loop.
- Mobile is progressing faster than desktop and needs the same features.
- A web build (deploy to Vercel, connect to the Go server, work from a browser) is wanted.
- A webview-based app is out (that's why MyGo/Electron-style stacks were rejected).

NSH is by Longbridge — the same team behind `gpui-component` — and answers exactly this split:
Rust for logic, TSX for screens, native platform views everywhere, no webview (except where you
ask for one).

## Verified from source

- **Styling**: real Tailwind 4 engine written in Rust (`crates/nsh-core/src/tailwind/`). `class`,
  `classList`, arbitrary values, `md:` breakpoints (follow the window), `dark:`, `active:`/`hover:`
  states tracked natively. shadcn/ui token set as the theme system. On web it emits real CSS.
- **Interop** (`#[native_module("app/native")]`): Rust exports components (typed props + events),
  function modules, and stateful classes to TSX. TypeScript declarations are generated from the
  Rust source (doc comments included) into `.nsh/types`; `nsh check` type-checks TSX against them.
  Sync fns return values, async fns return Promises, `Callback<A, R>` lets Rust call into TSX.
  Reference example: `examples/rust-exports` in the nsh repo.
- **Reactivity**: one shared graph — JS `signal()` is a native binding over the same
  Solid-style graph Rust components use. Boundary is props/events/callbacks, not raw signals.
- **Hot reload**: TSX edits patch the running app and preserve state; Rust edits rebuild + restart.
- **npm packages**: the bundler (oxc + rolldown, no Node needed) resolves `node_modules` and
  prefers packages' `browser` builds. Pure-JS packages work; React doesn't (no React runtime);
  DOM-touching packages don't ("Packages that touch the DOM do not work: there is none").
- **Built-in components** (`nsh/base`): Button, Dialog, Menu, Popover, Tabs, Command, DataTable,
  Tree, Combobox, Calendar, Toast, Tooltip, Sheet, NavStack, **Editor** (code editing + syntax
  highlighting), **DiffView** (split/unified, word highlights, collapsed unchanged lines),
  TextView (Markdown/HTML), Canvas, Shader, Video — plus navigation, dialogs, keyboard.
  `@pierre/diffs` is therefore not needed: `DiffView` covers it natively.
- **Testing** (`nsh test`): headless runs of the real UI, with `--trace` printing native view
  operations and per-commit timings; tests can press keys, submit inputs, and simulate the host
  app (`host.params`, `host.handlers`). Works across targets, including `--target web`.
- **Sandbox limits** (256 MB heap, 2 s call budget, node limits) apply only when embedding
  third-party signed TSX packages; a standalone app runs unlimited.

## Keyboard shortcuts (`nsh/keymap`)

- TSX: `keymap.bind("mod+k", handler)`, `keymap.bind(keystroke, handler, { when, repeat })`,
  `keymap.bind({ "mod+k": fn, ... })`; Rust: `on_key` / `on_key_with` / `key_label`.
- `mod` is per-platform: ⌘ on macOS/iOS, Ctrl on Windows/Linux/browsers — one spelling everywhere.
- `when` controls whether the shortcut fires while a text field has focus (`Auto`, `Always`,
  `NotEditing`); `repeat` controls key-repeat.
- Keystrokes are parsed and matched natively at key-press time (no JS round trip); the handler
  runs right after on the engine thread (Rust) or JS thread (TSX).
- `key_label("mod+k")` renders the platform-correct label (`⇧⌘L` vs `Ctrl+Shift+L`) for UI text.
- Menu items (`nsh/desktop`) take a `shortcut` too and show it in the platform's own style; menu
  shortcuts take precedence over focused fields; only a keymap binding of the same keystroke
  outranks them. System keys (Alt+F4, Cmd+Q, …) can't be bound.
- Works on macOS, Windows, Linux, web, and iOS/Android with a hardware keyboard.

## Web target

- `nsh package --target web` emits a static site (content-hashed files, precompressed `.br`/`.gz`,
  webmanifest, split bundles) — deployable to Vercel/Netlify/Cloudflare Pages as-is. Serve `.wasm`
  as `application/wasm`; prefer brotli.
- The Rust engine (and the app's Rust core) compile to WASM and run in the browser; TSX runs on the
  browser's JS engine. Basic TSX app runtime ≈ 1.1 MB.
- Server connection: `fetch`/WebSocket via the browser stack; relative URLs hit the serving origin;
  CORS applies cross-origin; `web.connect` in `nsh.json` whitelists hosts for the host module.
- Browser-only limitations are expected (no native-only features such as global shortcuts,
  single-instance, WKWebView cookie stores).

## Device streaming (Android emulators / iOS simulators)

Current stack: `sim-go` on the backend encodes AVCC H.264 and pumps it over a WebSocket to the
player page served at `GET /api/devices/player` (`internal/routes/device_player.go`); the page
decodes with WebCodecs onto a canvas and sends touch/scroll/key input back over the same socket.
Clients only host the page in a webview (`window.ipc.postMessage` is the sole app bridge).

Migration: `<WebView src={server}/api/devices/player">` in NSH, with an `injectedJavaScript` shim
`window.ipc = { postMessage: m => window.nsh.postMessage(m) }` — the player page itself is
unchanged. NSH's capability system gates the webview to the server host. Gaps: NSH's WebView has
no screenshot/snapshot API today; Android WebView needs a recent version for WebCodecs (the
existing Android app already plays this page, so this is proven in practice).

## Known gaps / risks

- NSH is v0.1.x — early-adopter risk; API may move.
- Linux desktop requires GTK 4.14+ (Ubuntu 24.04+); prebuilt binaries need glibc 2.39 — older
  distros build the CLI from source.
- No WebView screenshot API; cookie injection and other native browser plumbing must be added as
  Rust-side native components.
- Desktop menus/tray/global-shortcut API surface still needs a detailed check (`nsh/desktop`
  covers window chrome, app menu, context menus; tray/notifications unverified).
- No third-party component ecosystem; `nsh/base` + own components are the library.

## Demo

A TSX notes app lives at `~/apps/notes` in the orb (created with `nsh init --template ts`,
nsh 0.4.0 built from source): a sidebar of notes, an editor, and `mod+n` bound via `keymap` to
"New note". Verified in the orb: `nsh test` passes headless (including the `mod+n` shortcut
test), and the web build runs at `nsh run --target web` (port 8790, supervised as the `notes`
service, exposed through the portal) where clicks and real Ctrl+N presses were exercised in a
browser. On a machine with GTK 4.14+, `nsh run` in that directory opens the same app in a native
window.
