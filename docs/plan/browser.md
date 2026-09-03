# Embedded Native Browser Tab (GPUI + WKWebView) — Architecture & Plan

## 1. Executive Summary & Objective

This document analyzes the native browser implementation from [`waku/src/browser.rs`](file:///Users/adelodunpeter/Developer/Projects/waku/src/browser.rs) and details the blueprint for embedding a native browser tab directly into Console's right inspector panel.

When developing web apps or reviewing agent-generated projects, users currently have to leave Console and switch to external browsers. Adding an embedded native browser tab enables:
- **Instant Live Preview**: Auto-loading local development servers (e.g., `http://localhost:3000`, `http://localhost:5173`).
- **In-App Documentation & Web Browsing**: Browsing docs, API references, or web research side-by-side with the chat.
- **Embedded Web Inspector / DevTools**: Inspecting DOM, console errors, and network calls without leaving the workspace.

---

## 2. Key Architecture Insights from `waku`

Embedding a native webview (`WKWebView` on macOS, `WebView2` on Windows) inside a GPUI Metal/DirectX rendering tree involves three fundamental challenges that `waku` solved cleanly:

### A. The "Airspace" / Z-Ordering Problem
- **The Problem**: A native `NSView` (`WKWebView`) placed in an `NSWindow` is rendered directly by the OS window server. By default, AppKit composites it **on top of** the Metal view where GPUI draws. As a result, GPUI menus, tooltips, popovers, and the Command Palette (`⌘K`) get drawn underneath the webview and become invisible.
- **The `waku` Solution**:
  1. **Sublayer Ordering (`lower_below_scene_overlay`)**: GPUI renders menus and popovers onto a dedicated `GPUIOverlayView`. On macOS, `waku` traverses the window view hierarchy and anchors the `WKWebView` immediately below `GPUIOverlayView` using `superview.addSubview_positioned_relativeTo(view, NSWindowOrderingMode::Below, Some(&sibling))`.
  2. **Frozen Snapshotting**: When GPUI overlays open or during window occlusions, `waku` takes a fast asynchronous pixel snapshot (`takeSnapshotWithConfiguration`) and renders it as a GPUI `img(snapshot)` while hiding the live native view.

### B. Two Independent Focus Systems
- **The Problem**: AppKit has its `firstResponder` chain; GPUI has its own virtual `FocusHandle` tree. If the user clicks into a text input on a webpage, AppKit moves `firstResponder` to the webview. If the user then presses `⌘L`, clicks the address bar, or types into the chat composer, keystrokes will still go to the webview unless the native keyboard focus is reclaimed.
- **The `waku` Solution**:
  - **KVO on `NSWindow.firstResponder`**: A lightweight observer monitors AppKit's `firstResponder`.
  - **Gesture Edge-Detection**: Checks `NSApplication.currentEvent` to determine if a focus move was initiated by a user click versus a webpage script calling `input.focus()`.
  - **Reclaim on GPUI Focus Out**: When GPUI focus moves to the address bar, composer, or any other GPUI control, `reclaim_native_keyboard()` invokes `host.webview.focus_parent()`.

### C. Zero-Cost Geometry Synchronization
- **The Problem**: Layout runs every frame in GPUI. Pushing bounds to AppKit on every frame causes massive IPC/AppKit layout churn.
- **The `waku` Solution**:
  - Uses `gpui::canvas` in `render_page_area` to measure the layout bounds.
  - Logical point edge rounding prevents fractional pixel gaps/seams.
  - Bounds and scale factors are cached in a `Cell<Option<(i32, i32, i32, i32)>>` and deduplicated so unchanged layout frames cost 0 CPU cycles.

---

## 3. UI/UX Design for Console Desktop

### Location in Console
The browser will integrate as a third tab in Console's right sidebar (alongside **All files** and **Changes**):

```
┌────────────────────────────────────────────────────────────┐
│ Right Sidebar Header:                                      │
│ [ All files ]  [ Changes (N) ]  [ Browser ]     [ ⟳ Refresh]│
├────────────────────────────────────────────────────────────┤
│ Toolbar: [ ‹ ] [ › ] [ ⟳ ] [ 🔒 http://localhost:3000 ] [ ↗ ]│
├────────────────────────────────────────────────────────────┤
│                                                            │
│                                                            │
│                    Live Webview Surface                    │
│                                                            │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

### Toolbar Elements
1. **Navigation Controls**:
   - `Back` (`⌘[`): History backward (disabled when history is empty).
   - `Forward` (`⌘]`): History forward.
   - `Reload / Stop` (`⌘R`): Toggles between reload icon and stop loading `X` icon.
2. **Omnibox / Address Bar**:
   - Safari-style input with `display_url` (hiding `https://` clutter).
   - Security badge: Lock icon for HTTPS, globe for HTTP/localhost.
   - Linear loading progress bar embedded in the bottom of the address bar.
   - Smart resolution: schemes pass through; `localhost:3000` auto-prefixes `http://`; plain search terms route to Google.
3. **Utility Actions**:
   - `Open External` (`↗`): Launches the current URL in your default system browser (Safari, Chrome, etc.).
   - `DevTools`: Toggles WebKit Web Inspector for inspecting DOM, CSS, and Console logs.

---

## 4. Implementation Steps for Console

### Step 1: Add Dependencies
In `apps/desktop/Cargo.toml` and `crates/console-ui/Cargo.toml`:
```toml
wry = "0.47"
objc2 = "0.5"
objc2-app-kit = "0.2"
objc2-foundation = "0.2"
objc2-web-kit = "0.2"
image = "0.25"
```

### Step 2: Create `browser` Component Module
Create `apps/desktop/crates/console-ui/src/inspector/browser_view.rs`:
- Port `WebviewHost` and macOS AppKit KVO responder observer from `waku/src/browser.rs`.
- Implement `BrowserView` entity with address bar, navigation state, and progress bar.
- Hook into GPUI `canvas` layout for bounds sync.

### Step 3: Wire into Right Sidebar Inspector
In `apps/desktop/crates/console-ui/src/inspector/right_sidebar.rs`:
- Add `InspectorTab::Browser` to the segmented tab control.
- In `render()`, when active tab is `InspectorTab::Browser`, render the `BrowserView`.
- In `sync_native_state()`, pass visibility flags: if the tab is switched away or the right sidebar is collapsed, immediately hide the `WKWebView`.

### Step 4: Add Context Actions & Shortcuts
- Register global actions:
  - `FocusBrowserAddress` (`⌘L`)
  - `BrowserReload` (`⌘R`)
  - `BrowserBack` (`⌘[`)
  - `BrowserForward` (`⌘]`)
  - `BrowserDevtools` (`⌥⌘I`)

---

## 5. Verification & Testing Checklist

1. **Retina & High-DPI Scaling**:
   - Verify layout on Retina displays (2x scale) has crisp rendering and zero fractional pixel border seams.
2. **Overlay & Popover Layering**:
   - Open Command Palette (`⌘K`) while browser tab is active. Verify palette renders completely on top of the webview without occlusion.
3. **Keyboard & Focus Handoff**:
   - Click into Google search input inside the browser; type text.
   - Press `⌘L` or click the chat composer; verify keyboard immediately switches focus to the text field without getting trapped in the webview.
4. **Dev Server Live Reload**:
   - Start a Vite / Next.js dev server on `http://localhost:3000`.
   - Verify HMR (hot module replacement) and live reloads work seamlessly.
