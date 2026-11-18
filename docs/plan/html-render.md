# HTML render — interactive pages inline in chat

Agents answer with a real HTML page instead of only text: charts, mockups, dashboards, and fully interactive widgets (the sidebar-design prototype is the canonical example). The page is a **workspace file**, so it can be previewed, screenshotted, edited, and reopened — not a frozen blob.

Reference implementation studied: t3code's `html_render`/`html_preview` pair. Console's version differs deliberately: pages are workspace files served through the fs routes, verification reuses the desktop browser bridge instead of a downloaded headless Chrome, and the artifact card is a snapshot that expands into a live sandboxed webview.

---

## 0. Decisions (settled in discussion)

- **HTML replaces the `visualize` spec entirely — no backward compatibility.** The model writes HTML it already knows; no spec grammar to learn. The `visualize` tool, the `/visualize` capability, and the spec painters/parsers are all removed; old transcripts with spec cards render the existing unsupported-spec fallback. `image` blocks produced by `read_file` are unaffected — they are a different producer of `VisualArtifact` and stay.
- **The artifact is a workspace file.** `html_render` accepts `{html}` or `{path}`. Inline HTML is written to a file under the session's scratch area; `{path}` references an existing workspace file read-only. Either way the card, the file preview, and the `.html` chooser all render the same bytes.
- **Card = snapshot → live webview.** Collapsed state is a PNG snapshot (captured during `html_preview`, or lazily on first expand); expanding mounts a sandboxed `WebviewHost` inline in the transcript. "Open in browser tab" and "Open externally" are card actions. Fullscreen reuses the existing expand affordance.
- **Verification rides the existing browser bridge.** `html_preview` opens the file in a kernel-owned browser tab, returns a screenshot PNG + measured `contentHeight` + console/page errors, and can click/type to verify interactivity. No headless Chrome download — the user's own WKWebView does the rendering, so it is already theme-accurate.
- **Pages get theme variables.** A bootstrap fragment (injected at serve time, not baked into the file) defines `--background`, `--foreground`, `--accent`, `--chart-1..6`, `--radius`, `--font-sans`, `--font-mono` from the client's current theme, plus a `postMessage` bridge for height reporting, link clicks, and (phase 2) page→agent messages.
- **`.html` files get a chooser everywhere.** Clicking an `.html`/`.htm` file — file tree, quick open, chat file link — offers Preview (sandboxed) vs Editor vs External browser instead of silently opening the editor. Mobile gets the same three choices.
- **Desktop first.** Android ships the card as snapshot + "Open preview" (sandboxed WebView screen) in phase 2; inline live webview in the transcript is a later phase.

---

## 1. What Already Exists (verified)

- **Wire**: `ToolResult.visual` → `VisualPart{version, spec}` (`proto/console/v1/message.proto:121`). The comment already anticipates versioned client-rendered artifacts. `tools.VisualArtifact` + `tools.Envelope.Visual` carry it loop-internally (`agent/tools/tool.go:33`); `toolResultToProto`/`protoToToolResults` (`agent/loop/proto.go`, `run/wire.go`) and the DB split/reassemble path (`agent/loop/decode.go`, `msgparts`) round-trip it.
- **Desktop card**: `console-ui/src/chat/visual/` paints spec blocks; `row.rs:137` collects `result.visual` per tool call and renders `VisualCard` after the run block.
- **Android card**: `VisualArtifact` model (`data/model/ToolModels.kt:66`), proto decode in `MessageConverters.kt:43`, `RunVisuals` in `feature/chat/RunActivity.kt:259`, painter in `feature/chat/visual/`. All of this is removed with the spec — except the `image` block path (see removal scope in §9).
- **Sandboxed webview**: `console-ui/src/browser/host.rs` — `WebviewHost::create` takes `HostContent{html, initialization_script, allow_navigation, on_ipc, on_new_window_url}`; wry backend maps to `WebViewBuilder`. `with_html` + navigation denial + new-window denial is the whole sandbox on macOS (WKWebView). Non-macOS falls back to the stub host — HTML cards degrade to snapshot + "open externally" there.
- **Browser bridge for verification**: `BrowserActionRequest` already has `Action`, `URL`, `Script`, `TabID`, `NewTab` and `BrowserActionResult.ImageBase64` (`agent/tools/browser_wire.go`). Desktop handles `"screenshot"` (`apps/desktop/src/state/browser_actions.rs:763`, returns PNG via `WebviewHost::take_snapshot`) and `"run_js"` (`driver.js` `eval` op). The browser-use kernel (`services/browseruse/js.go`) exposes open/goto/snapshot/content/query/click/type/waitFor/extract/scroll/paginate/find/close — **no `screenshot` or `runJs` method yet**; both are wire-supported already.
- **File serving**: `GET /api/fs/file/raw` (bytes + ETag, `routes/fs.go:280`) and `/api/fs/file/media` (Range, token-in-query allowed for webviews, `device_auth.go:50`). Neither serves `.html` with a CSP today — `GetImageMeta` won't classify it.
- **Workspace tabs**: `WorkspaceTabConfig::File` (editor) and `::Browser` (browser tab, `types/workspace.rs:60`) — both destinations for the `.html` chooser already exist.
- **Android file kinds**: `filePreviewKind()` (`data/model/FsModels.kt:86`) maps `.html` → `TEXT` (editor). `BrowserWebView` composable + `HttpTransport.mediaUrl` (token-in-query) exist for previews.
- **Theme**: `console-ui/src/theme/mod.rs` `Theme` has every token needed (canvas, text, accent, chart[8], radius-equivalents, code_text). Android has `ThemeNew.kt`; the variable mapping is trivial.

---

## 2. Server (server-go)

### 2.1 New package `internal/services/htmlrender`

Mirror `services/visualize` structure: `tool.go`, `skill.go` (embedded SKILL.md), `invoke.go`. Register as a new `Capability` entry `html` in `run/capabilities.go` with `AgentMayEnable: true`, `AlwaysLoaded: true`, `TierRead`, `GatedTools: ["html_render","html_preview"]` — same shape as `visualize`.

### 2.2 Tools

```json
html_render { "path": "optional .html path",
              "html": "optional inline HTML (≤ 512 KiB)",
              "title": "optional card title (≤200 chars)",
              "height": "optional frame height px, 80–2000" }
```

- Exactly one of `path`/`html` required. `path` must resolve inside the workspace (reuse `fssvc.ResolvePath` rules) and start with `<!doctype`/`<html` after trim — same sniff t3code uses so a renamed `.env` can't be published.
- Inline `html` is written to a scratch file (`.console/renders/<slug>.html` under the workspace, or the session store dir when there is no workspace — decide at impl, document in SKILL.md).
- Size caps: inline ≤ 512 KiB; file ≤ 4 MiB.
- Reject `vh`/`vw` units in `<style>`/`style=` (t3code's `rejectViewportHeight` — copy the regex approach; the frame auto-fits height so viewport units loop).
- Local image inlining: copy t3code's `inlineLocalImages` approach — find absolute-path image refs, verify magic bytes (PNG/JPEG/GIF/WebP/AVIF/SVG-root check), inline as data URIs, cap total page at the file limit. **Phase 2** — v1 can serve images through the same sandboxed route if they live under the workspace; inline-HTML pages with local images are the gap.
- Returns `Envelope{Content: summary text, Html: &HtmlArtifact{...}}` — see §3.

`html_preview` (same gating):

```json
{ "path"?: "...", "html"?: "...", "width"?: 728, "height"?: 800,
  "click"?: {"text"|"selector"|"ref": "..."}, "type"?: {"text","selector"|"ref"} }
```

- Prepares the page exactly like `html_render` (same validation + bootstrap injection) but **does not publish**: writes to a temp/scratch location, opens it in a kernel-owned browser tab via the existing `BrowserHandler` (`Action:"navigate"`, `NewTab:true`), waits, then:
  - `Action:"screenshot"` → PNG → return as an image content part (the model sees its page).
  - `Action:"run_js"` → `document.documentElement.scrollHeight`, `document.title`, collected `window.__errors` (injected hook: `window.onerror` + `console.error/warn` capture installed by the bootstrap before page scripts run).
  - Optional `click`/`type` run one action and re-screenshot — this is the "verify interactivity" superpower over t3code.
- Returns `{screenshot, contentHeight, title, consoleMessages[], missingImages?}` as structured content + the PNG as an image part.
- Requires the desktop app connected (browser bridge). When no desktop is attached, return a clear tool error: "preview needs the desktop app; publish anyway with html_render" — publish must never depend on preview.

### 2.3 Proto — `message.proto`

```proto
// HtmlPart is a client-rendered HTML page artifact. Path is workspace-relative
// or absolute; clients fetch it through the sandboxed html route. Snapshot is
// an optional base64 PNG for the collapsed card state.
message HtmlPart {
  string path = 1;
  string title = 2;
  uint32 height = 3;           // agent-declared frame height, clamped client-side
  optional bytes snapshot = 4; // PNG, absent until first preview/expand
}
```

`ToolResult` gains `optional HtmlPart html = 6;` (field 5 is `visual`). Regenerate: `make proto` (buf) + desktop prost build picks it up automatically; Android Wire regenerates from the same `.proto`.

### 2.4 Serving — `GET /api/fs/file/html?path=...`

New route beside `/file/raw` in `routes/fs.go`:

- Auth: normal bearer; **also accept `?token=` for GET** like `/file/media` (webviews can't set headers) — extend `mediaFilePath`'s exemption or add `htmlFilePath`.
- Validate: extension `.html`/`.htm`, file ≤ 4 MiB, content starts like a document.
- Response: `Content-Type: text/html; charset=utf-8`, `Content-Security-Policy: sandbox allow-scripts allow-forms allow-popups allow-downloads` (copy t3code's — opaque origin, scripts allowed, no same-origin ever), `X-Content-Type-Options: nosniff`, `Cache-Control: private, max-age=30` + ETag like siblings.
- The bootstrap (theme vars + postMessage bridge + error hook) is **injected at serve time** into `<head>`, not written into the file — the file on disk stays clean and editable. Injection must blank non-markup regions first (t3code's `blankNonMarkup` — copy it; naive injection into `<script>` strings corrupts pages).
- Range not needed; single GET.

### 2.5 Theme fragment

`?theme=dark|light` query param selects which variable set the bootstrap emits; clients pass their current appearance. Live theme *changes* while a card is expanded go through the postMessage bridge (`t3/notifications/theme`), same pattern as t3code.

---

## 3. Wire types (server-internal)

`tools.ToolResult` gains `Html *HtmlArtifact` alongside `Visual`:

```go
type HtmlArtifact struct {
    Path     string `json:"path"`     // workspace path of the .html file
    Title    string `json:"title"`
    Height   uint32 `json:"height"`
    Snapshot []byte `json:"snapshot,omitempty"` // PNG
}
```

- `Envelope` gains `Html *HtmlArtifact`; executor passes it through like `Visual`.
- `toolResultToProto` (`run/wire.go`) + `toolResultsToProto`/`protoToToolResults` (`agent/loop/proto.go`) + `decode.go` rawToolResult + `run/turns.go` subagent exclusion (exclude `html_render`/`html_preview` from subagent tool source, same as `visualize`) — every place `Visual` is touched gets an `Html` sibling.
- Persistence: the artifact is tiny (path + optional snapshot). Snapshot PNGs are the only bulk — route them through `msgparts` externalization like images (reuse `KindImage`; the kind only matters for sweep semantics and image sweep already handles "referenced by parts"). **Verify** the sweep's `isImageKind` gate covers the chosen kind.
- Provider-facing content stays a short text summary ("Rendered interactive page: title (path)") — HTML never goes to the model provider, same rule as `VisualPart`.

---

## 4. Desktop (GPUI)

### 4.1 Card — `console-ui/src/chat/html/` (new module beside `visual/`)

- `row.rs` collects `result.html` the same way it collects `result.visual` (same loop, second artifact kind).
- Collapsed: snapshot image if present, else a titled placeholder ("Interactive page — click to load"). Actions row: **Expand**, **Open in browser tab** (`WorkspaceTabConfig::Browser` pointed at `/api/fs/file/html?path=…&token=…`), **Open externally**.
- Expanded: mount `WebviewHost` at measured height (postMessage `size-changed` → auto-fit up to 2000px, scroll inside beyond). `HostContent`: navigate to the route URL so the page keeps its own URL semantics.
- `allow_navigation`: allow only `http(s)` navigations to external sites → forward to system browser and deny in-webview; deny everything else. `on_new_window_url`: same forward + deny.
- `on_ipc`: handle `t3/notifications/size-changed` (height), `t3/notifications/open-link` (external open). Theme changes go out via `evaluate_script`.
- Theme: on expand and on theme change, inject the theme fragment (JSON of CSS vars) — the bootstrap listens for it.
- **Self-contained pages for v1**: relative refs inside HTML resolve against `/api/fs/file/html` and break, so SKILL.md requires inline CSS/JS and images via data URI or absolute `/api/fs/file/raw?path=…&token=…` URLs. Relative-asset support is a follow-up (§9).
- Non-macOS (stub `WebviewHost`): card shows snapshot/placeholder + "Open in browser" only. No crash, no dead webview.
- Keyboard/a11y: card is a normal transcript element; the webview takes focus only when clicked (`with_focused(false)` + existing responder handling).

### 4.2 `.html` file chooser

- Find file-open call sites: file tree double-click, quick open, chat file-link clicks, "open file" actions. For `.html`/`.htm`, show a small menu (GPUI dropdown/popover — check `primitives` for existing menu component) with **Preview** (opens `WorkspaceTabConfig::Browser` at the html route URL — full browser chrome; the route's CSP still applies), **Editor** (existing `File` tab), **External browser**.
- Always ask (one click); remembering the choice is optional polish, skip for v1.
- Chat file links to `.html` get the same chooser.

---

## 5. Android

### 5.1 Card (phase 2)

- Proto: Wire regenerates `HtmlPart`; `ToolResult` model gains `html: HtmlArtifact?` (`path`, `title`, `height`, `snapshot` bytes→base64 PNG).
- `RunVisuals` renders an `HtmlCard`: snapshot image + title + **Open preview** button → new `HtmlPreviewScreen`: `WebView` with `javaScriptEnabled=true`, `domStorageEnabled=false`, `allowFileAccess=false`, `allowContentAccess=false`, `mixedContentMode=NEVER_ALLOW`, loading the server URL + `?token=` (mediaUrl pattern). `shouldOverrideUrlLoading`: allow only the page URL + `#`; external links → `Intent.ACTION_VIEW`. Actions: "Open in editor" (file screen), "Open externally".
- Inline live webview in the transcript: **defer** — WebView-in-LazyColumn has real recycling/height problems; snapshot + preview screen covers v1.

### 5.2 `.html` chooser

- `filePreviewKind()`: `.html`/`.htm` → new `FilePreviewKind.HTML`.
- `FilesScreen` HTML branch: preview pane with the sandboxed WebView + top-bar actions **Editor** / **External browser**. Tapping the file opens preview by default (it's a page, not code); editor is one tap away. (Inverts today's TEXT default deliberately.)

---

## 6. Verification loop (the "better than t3code" part)

`html_preview` returns screenshot + metrics + console errors **and can interact**: `{click:{text:"Submit"}}` then re-screenshot proves a button works, not just that it renders. t3code's headless Chrome can only look; ours drives the real engine the user sees.

Kernel additions (`services/browseruse/js.go`):

- `browser.screenshot()` → `callAction({Action:"screenshot", TabID})` → decode `ImageBase64` → return as image part. Wire + desktop already implemented.
- `browser.runJs(code)` → `callAction({Action:"run_js", Script:code})` → returns `Result` string. Wire + desktop already implemented.
- `html_preview` tool uses `BrowserHandler` directly (not the JS kernel) for its fixed sequence: navigate → wait → screenshot → run_js(metrics+errors) → optional click/type → re-screenshot. Simpler and more reliable than asking the model to script it.
- Console capture: bootstrap installs `window.__consoleErrors` before page scripts (serve-time injection happens pre-parse, so this works for the preview route).

---

## 7. Security checklist

- [ ] CSP `sandbox` (no `allow-same-origin`) on every HTML response — page gets opaque origin, no cookies/storage/API.
- [ ] Tighten resource policy beyond bare sandbox: decide exact `default-src`/`script-src`/`img-src`/`connect-src` at impl; t3code relies on iframe sandbox + opaque origin, we should match or tighten.
- [ ] `allow_navigation` denies in-webview navigation; external links forwarded to system browser.
- [ ] New-window requests denied.
- [ ] File path validation: workspace-confined, `.html` extension, size cap, document-start sniff.
- [ ] Token-in-query only on the GET html route (same exemption class as `/file/media`).
- [ ] No secrets in pages: SKILL.md instructs no credentials.
- [ ] `vh`/`vw` rejection prevents auto-fit loops.
- [ ] Subagent exclusion for both tools.

---

## 8. Testing

- `services/htmlrender`: validation table tests (path vs html, sniff, size, vh rejection), bootstrap injection (blankNonMarkup cases), image inlining magic-byte checks.
- Route test: `/api/fs/file/html` returns CSP header, 404/400 on bad path, token query auth.
- Proto round-trip: `HtmlPart` fixture in `proto/testdata` + decode tests (Go `protoToToolResults`, Rust `message_json_test`, Android fixture test).
- Desktop: card states (placeholder/snapshot/live), navigation denial, theme injection — GPUI test or manual; non-macOS stub path.
- Browser bridge: `screenshot`/`run_js` already covered by existing action tests; add kernel method tests in `browseruse/kernel_test.go` style.
- Android: `filePreviewKind` HTML mapping fixture test; WebView settings unit-testable parts.

---

## 9. Non-goals / later

- Page→agent postMessage bridge (page sends a message into the composer) — designed-for in the bootstrap protocol, implemented later.
- Relative-asset resolution inside pages (v1: self-contained pages; images via data URI or absolute fs URLs).
- Inline live WebView in the Android transcript.
- `html_preview` on mobile (needs desktop bridge; mobile gets publish-only).
- (Removed from non-goals: spec painters ARE removed — no backward compat.)
- **Removal scope**: delete `services/visualize` (tool, SKILL.md, loader), the `visualize` Capability entry, `console-ui/src/chat/visual/` spec painters (keep `chat/image.rs` — it also serves markdown/workspace images), `console-core` `parse_visual`/spec types, Android `feature/chat/visual/` + `VisualSpec.kt`. `VisualArtifact`/`VisualPart` plumbing stays only if `read_file` images still need it — **decision**: migrate `read_file` image visuals to render through the same image path markdown images use (`chat/image.rs` workspace-image fetcher), then drop `VisualArtifact` entirely. Old transcripts show the unsupported-spec fallback.
- MCP-app-style tool calling from inside pages.

## 10. Suggested phasing

1. **Server + proto**: `HtmlPart`, tools, route, bootstrap, persistence. Card renders as placeholder/snapshot on desktop.
2. **Desktop card**: snapshot → lazy webview, chooser for `.html` files, browser-tab/external actions.
3. **Verification**: kernel `screenshot`/`runJs` methods + `html_preview` tool.
4. **Android**: card + preview screen + file chooser.
5. **Polish**: page→agent bridge, relative assets, theme-live-update polish.