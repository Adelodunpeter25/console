# Embedded Native Browser Tab (GPUI + `gpui-wry`) — Architecture & Plan

## 1. Executive Summary & Objective

This document analyzes the native browser implementation from [`waku/src/browser.rs`](file:///Users/adelodunpeter/Developer/Projects/waku/src/browser.rs) and defines a plan for embedding a native browser surface directly into Console's right inspector panel.

When developing web apps or reviewing agent-generated projects, users currently have to leave Console and switch to an external browser. An embedded browser would provide:

- **Instant live preview**: Load local development servers such as `http://localhost:3000` and `http://localhost:5173`.
- **In-app documentation and browsing**: View documentation, API references, and web research beside the chat.
- **Embedded inspection**: Provide access to browser developer tools where the platform supports it.

The first implementation should be a macOS-focused localhost preview with basic navigation. General web browsing, downloads, popups, and full developer-tool integration should be added only after the native embedding and focus behavior are stable.

---

## 2. Recommended Webview Foundation: `gpui-wry`

### 2.1 What it provides

[`gpui-wry`](https://crates.io/crates/gpui-wry) is an experimental GPUI integration built on [Wry](https://github.com/tauri-apps/wry). Wry uses the platform's native webview implementation, including `WKWebView` on macOS and WebView2 on Windows.

The current published version is `0.6.0` (API documentation: [`docs.rs/gpui-wry`](https://docs.rs/gpui-wry/latest/gpui_wry/)). The source is maintained in [`longbridge/gpui-kit`](https://github.com/longbridge/gpui-kit/tree/main/crates/webview).

The crate exposes:

- `gpui_wry::WebView`: A GPUI-renderable wrapper around a Wry webview.
- `gpui_wry::WebViewElement`: A GPUI element that positions the native webview using the element's layout bounds.
- `gpui_wry::WebViewHandle`: A cloneable, UI-thread-local handle to the underlying Wry webview.

The wrapper already includes the basic layout and lifecycle behavior needed by Console:

- Measures the GPUI element with a canvas and synchronizes native bounds through Wry.
- Converts GPUI bounds to Wry logical coordinates.
- Shows and hides the native view.
- Moves focus back to the parent window with `focus_parent()` when the view is hidden or the surrounding GPUI surface is clicked.
- Exposes the raw Wry API for URL loading, script evaluation, navigation, and platform-specific configuration.

This lets Console focus on browser state and product behavior rather than reimplementing the basic Wry-to-GPUI bridge.

### 2.2 Limitations and risks

`gpui-wry` is not a complete browser component. It does not provide the address bar, browser history model, loading UI, security policy, DevTools button, or project-specific preview discovery.

Its documented limitations are important for this design:

- It is explicitly experimental.
- It currently supports macOS and Windows, not Linux.
- The native webview renders above GPUI content inside its bounds. GPUI menus, popovers, tooltips, and command palettes behind it can be occluded.
- `WebView` and `WebViewElement` are UI-thread-local and must not be moved across threads.
- A retained `WebViewHandle` keeps the native webview alive and must be dropped before its parent window is destroyed.

The current Console workspace uses GPUI directly from the Zed repository, while `gpui-wry 0.6.0` documents dependencies on `gpui-pre` and `lb-wry`. Compatibility must therefore be verified before adopting the crate. We must avoid resolving two incompatible GPUI versions.

### 2.3 How it changes this plan

Use `gpui-wry` for the first implementation instead of porting the entire native `WebviewHost` from `waku`. Keep the `waku` techniques as fallback or supplemental implementation guidance for the parts `gpui-wry` does not solve:

- Native z-ordering relative to GPUI overlays.
- Snapshotting while overlays are open or the window is occluded.
- Detailed AppKit first-responder observation and gesture edge detection.
- Any macOS-specific WebKit or Web Inspector configuration.

The first technical milestone is a small compatibility spike that embeds a `gpui_wry::WebView` in the existing desktop window and verifies compilation, sizing, visibility, focus handoff, and overlay behavior.

---

## 3. Architecture Insights from `waku`

Embedding a native webview in a GPUI Metal/DirectX rendering tree still involves three important challenges.

### A. The "Airspace" / Z-Ordering Problem

A native `NSView` (`WKWebView`) is composited by the operating system and normally appears on top of the Metal view where GPUI draws. As a result, GPUI menus, tooltips, popovers, and the Command Palette (`⌘K`) can be drawn underneath the webview.

The `waku` implementation addresses this by:

1. Anchoring the webview below the dedicated GPUI overlay view using AppKit subview ordering.
2. Taking an asynchronous native snapshot and rendering it as a GPUI image while hiding the live webview whenever an overlay or occlusion requires it.

`gpui-wry` does not remove this limitation; the overlay strategy remains Console's responsibility. The implementation should begin with a simple z-order test, then add view ordering or snapshot fallback if required.

### B. Two Independent Focus Systems

AppKit has a `firstResponder` chain while GPUI has its own `FocusHandle` tree. Clicking a webpage input transfers native focus to the webview. Moving to the address bar, chat composer, or another Console control must reclaim native focus or keystrokes may remain trapped in the page.

The `waku` implementation uses:

- A lightweight observer for `NSWindow.firstResponder`.
- Current-event inspection to distinguish user clicks from webpage scripts calling `input.focus()`.
- `focus_parent()` when GPUI focus moves away from the webview.

`gpui-wry` provides `focus()` and `focus_parent()`, which should cover the basic handoff. The AppKit observer should only be added if the compatibility spike demonstrates that the simpler behavior is insufficient.

### C. Geometry Synchronization

Layout runs frequently in GPUI, so pushing native bounds on every frame can create unnecessary AppKit churn. `gpui-wry` already uses a canvas-based layout element and updates the native Wry bounds from the measured GPUI rectangle. Console should add bounds caching or deduplication if profiling shows repeated unchanged updates are expensive.

Retina/high-DPI handling and logical-point edge rounding must still be verified in the application.

---

## 4. UI/UX Design for Console Desktop

### Location in Console

The current right inspector already contains **All files**, **Changes**, and **Subagents**. Browser should therefore be added as a **fourth tab**, unless Subagents is intentionally moved or removed in a separate design update.

```text
┌────────────────────────────────────────────────────────────┐
│ [ All files ] [ Changes (N) ] [ Subagents ] [ Browser ]     │
│                                                   [Refresh] │
├────────────────────────────────────────────────────────────┤
│ [ ‹ ] [ › ] [ ⟳ ] [ 🔒 http://localhost:3000 ] [ ↗ ] [Dev]│
├────────────────────────────────────────────────────────────┤
│                                                            │
│                    Native Webview Surface                  │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

### Initial browser behavior

- Keep one persistent browser surface per desktop window.
- Preserve the current URL while switching between inspector tabs.
- Hide the native view when Browser is inactive, the sidebar is collapsed, or the window is being replaced.
- Initially load a blank page or an explicitly configured preview URL; do not guess a dev-server port without a discovery/configuration rule.
- Defer multiple browser tabs, downloads, popups, and unrestricted browsing until the base preview flow is reliable.

### Toolbar Elements

1. **Navigation controls**:
   - `Back` (`⌘[`): History backward; disabled when unavailable.
   - `Forward` (`⌘]`): History forward; disabled when unavailable.
   - `Reload / Stop` (`⌘R`): Reload the page or stop an active load.
2. **Address bar**:
   - Display the current URL, with optional presentation-only hiding of `https://`.
   - Show HTTPS and HTTP/localhost indicators.
   - Show loading progress when it is available from the native webview callbacks.
   - Resolve `localhost:3000` to `http://localhost:3000`. Keep search-engine routing configurable rather than hard-coding Google.
3. **Utility actions**:
   - `Open External`: Open the current URL in the system browser.
   - `DevTools`: Open Web Inspector where supported and configured; otherwise disable or explain the limitation.

---

## 5. Implementation Steps for Console

### Step 1: Validate dependency compatibility

Add `gpui-wry` to the smallest appropriate desktop crate after checking its GPUI compatibility. Do not add all of `wry`, `objc2`, and WebKit bindings by default; `gpui-wry` already brings the Wry integration, and direct AppKit dependencies should be added only for a demonstrated platform-specific requirement.

Validate:

- The current GPUI revision and `gpui-wry` use compatible GPUI APIs.
- Cargo resolves one compatible GPUI stack.
- macOS builds with the repository's deployment target.
- The webview can be created and destroyed on the GPUI/UI thread.

### Step 2: Build a compatibility spike

Create a minimal desktop-only example or temporary browser component that:

- Creates a Wry webview and wraps it in `gpui_wry::WebView`.
- Renders `WebViewElement` inside the existing window.
- Loads `http://localhost:3000` or a known test page.
- Updates bounds during sidebar resizing.
- Hides and shows the view without leaving focus trapped in it.
- Tests whether the command palette and other GPUI overlays appear above it.

Do not proceed with the full browser UI until these behaviors work or the fallback strategy is documented.

### Step 3: Create the browser component

Create `apps/desktop/crates/console-ui/src/inspector/browser_view.rs` for the browser state and GPUI toolbar. Keep native webview creation and window-specific integration in the desktop application layer if `console-ui` cannot own platform handles cleanly.

The component should own or receive:

- A `gpui_wry::WebView` entity or an appropriate UI-thread-local handle.
- Current URL and display URL.
- Loading/progress state.
- Back/forward availability.
- Navigation error state.
- Visibility and active-session/project context.
- Callbacks for address submission, navigation, external opening, and DevTools.

### Step 4: Wire the right inspector

Update `InspectorTab` and every exhaustive match to include `Browser`, including:

- `apps/desktop/crates/console-ui/src/inspector/right_sidebar.rs`.
- `apps/desktop/src/state/right_sidebar.rs` refresh behavior.
- `apps/desktop/src/view/mod.rs` tab callbacks and rendering.
- Any persistence or session/project state that should remember the selected tab.

When Browser is inactive or the right sidebar is collapsed, call the webview's `focus_parent()` and `set_visible(false)`. When it becomes active, update its bounds and call `set_visible(true)`.

### Step 5: Add browser commands and shortcuts

Register actions with precedence that does not break the composer or web content:

- `FocusBrowserAddress` (`⌘L`) when Browser is active.
- `BrowserReload` (`⌘R`) when Browser is active and no more specific text-input binding claims it.
- `BrowserBack` (`⌘[`), when Browser is active.
- `BrowserForward` (`⌘]`), when Browser is active.
- `BrowserDevtools` (`⌥⌘I`), when supported.

The address bar must explicitly reclaim native focus before accepting text. Moving to the chat composer must always reclaim focus from the webview.

### Step 6: Define security and navigation policy

Before enabling general browsing, document and implement policy for:

- JavaScript and initialization scripts.
- Localhost and file URL access.
- Navigation failures and redirects.
- New windows and popups.
- Downloads.
- Cookies and persistent storage.
- Authentication and permission prompts.
- External links and custom URL schemes.

For the first milestone, allow only normal HTTP/HTTPS navigation and localhost preview URLs, and open unsupported popup/download behavior externally or block it clearly.

### Step 7: Resolve overlay behavior

Start with the behavior provided by `gpui-wry` and test it against Console's overlay system. If the native webview occludes GPUI overlays:

1. Add platform-specific native subview ordering where possible.
2. Otherwise hide the webview and render a recent snapshot while a GPUI overlay is open.
3. Restore the live view and synchronize bounds after the overlay closes.

Document any limitations for menus, tooltips, command palette, window occlusion, and resizing.

---

## 6. Verification & Testing Checklist

1. **Dependency and lifecycle**:
   - Build with the repository's pinned GPUI revision.
   - Create and destroy the webview repeatedly without leaks or shutdown crashes.
   - Close the desktop window while navigation is active.
2. **Retina and high-DPI scaling**:
   - Verify crisp rendering at 2x scale and zero fractional-pixel seams.
   - Resize the sidebar repeatedly and confirm the native bounds track the GPUI surface.
3. **Overlay and popover layering**:
   - Open Command Palette (`⌘K`) while Browser is active.
   - Open menus, tooltips, and dialogs over the browser.
   - Verify the chosen ordering or snapshot fallback works without stale content.
4. **Keyboard and focus handoff**:
   - Click into a webpage input and type.
   - Press `⌘L` and verify the address bar receives input.
   - Click the chat composer and verify the webview no longer receives keystrokes.
   - Switch inspector tabs and collapse/reopen the sidebar while the page has focus.
5. **Navigation**:
   - Test back, forward, reload, stop, redirects, invalid URLs, and unreachable localhost servers.
   - Confirm URL and loading state update after navigation.
   - Confirm external opening uses the current URL.
6. **Development preview**:
   - Start a Vite or Next.js server on `http://localhost:3000`.
   - Verify HMR and live reload work.
   - Test changing the active project/session and confirm the documented URL persistence behavior.
7. **Platform behavior**:
   - Verify DevTools behavior on macOS.
   - Record Windows compatibility requirements if cross-platform support is enabled.
   - Explicitly document that Linux is unsupported by the current `gpui-wry` integration.

---

## 7. Phased Delivery

### Phase 1: macOS localhost preview

- Validate GPUI compatibility.
- Embed one persistent `gpui-wry` webview.
- Add Browser as the fourth inspector tab.
- Implement URL entry, reload, basic back/forward, visibility, resizing, and focus handoff.

### Phase 2: Console integration hardening

- Add navigation callbacks, progress, errors, external links, and project/session behavior.
- Resolve overlay z-ordering and snapshot fallback.
- Add lifecycle, security, and performance tests.

### Phase 3: Advanced browser capabilities

- Add configured preview-server discovery.
- Add Web Inspector/DevTools where supported.
- Decide on downloads, popups, cookies, authentication, general browsing, and multiple browser tabs.
- Evaluate Windows support after macOS behavior is stable.
