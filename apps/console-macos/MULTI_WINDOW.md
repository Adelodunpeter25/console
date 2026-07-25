# Console macOS — Multi-Window Architecture

## Requirement

The app must support **multiple independent windows**, each with its own
complete UI state. State must **not bleed** between windows.

Concretely:

- One window can have the sidebar collapsed while another has it expanded.
- One window can be on session A while another is on session B.
- One window can be in dark mode while another is in light mode.
- Closing one window must not affect any other window.
- Opening a new window (⌘N or dock → New Window) must start fresh — no
  inherited selection, scroll position, sidebar state, or settings sheet
  state from the window that spawned it.

Each window is a fully isolated workspace. The only shared state is the
backend connection itself (server URL, auth tokens, provider catalog),
which is global and intentionally shared across all windows.

## What must be per-window (isolated)

| State | Per-window | Notes |
|-------|-----------|-------|
| Selected session | ✅ | Each window picks its own session from the sidebar. |
| Sidebar collapsed/expanded | ✅ | `NavigationSplitView` visibility is window-local. |
| Sidebar width | ✅ | Column width drag is per-window. |
| Active session VM | ✅ | Each window owns its own `SessionViewModel` for its selected session. |
| Scroll position | ✅ | Transcript scroll is per-window. |
| Streaming state | ✅ | `isStreaming`, `streamingText`, etc. live on the window's session VM. |
| Settings sheet open/closed | ✅ | Sheet presentation is window-scoped. |
| Appearance (light/dark) | ✅ | If per-window appearance is supported, each window sets its own `NSAppearance`. |
| Theme | ✅ | Theme injection happens at each window root, not at app root. |

## What is shared (global)

| State | Shared | Notes |
|-------|--------|-------|
| Server URL | ✅ | One backend; changing it in settings affects all windows. |
| Auth status / tokens | ✅ | Login state is global. |
| Provider catalog | ✅ | Model list is fetched once, shared. |
| Session list | ✅ | All windows see the same sessions (same backend). Creating a session in one window should refresh the list in others. |
| Project list | ✅ | Same as sessions. |

## Implementation guide

### Current state

Today the app uses SwiftUI `WindowGroup` with a single `ContentView` that
creates its own `@StateObject AppViewModel`. SwiftUI's `WindowGroup` already
creates a fresh view tree per window, so `@State` and `@StateObject` inside
`ContentView` are naturally per-window. However, `AppViewModel` holds both
global state (auth, providers, server URL) and session list state — and each
window creates its own instance, so session list changes in one window don't
propagate to another.

### Target architecture

1. **Split `AppViewModel` into global vs. window-local state.**
   - `AppGlobalState` — shared singleton (`@MainActor`, `ObservableObject`):
     server URL, auth status, provider catalog, project list. Injected via
     `.environmentObject` at the app/scene level so every window reads the
     same instance.
   - `WindowViewModel` — created per-window (`@StateObject` in `ContentView`):
     selected session ID, sidebar visibility, settings sheet state, the
     current `SessionViewModel`. This is the state that must not bleed.

2. **`WindowGroup` handles multi-window creation.** SwiftUI `WindowGroup`
   already instantiates a separate view tree per window. As long as
   `WindowViewModel` is a `@StateObject` inside `ContentView` (not
   `@EnvironmentObject`), each window gets its own instance automatically.

3. **Session list refresh across windows.** When a window creates or deletes
   a session, it should call `AppGlobalState.refreshSessions()`, which
   triggers a `@Published` update. Since `AppGlobalState` is a shared
   singleton injected via `.environmentObject`, all windows re-render their
   sidebars. Each window's selection (`selectedSessionId`) stays independent.

4. **Per-window appearance (optional).** Set `window.appearance()` on the
   `NSWindow` instance via an `NSWindowAccessor` helper if per-window
   light/dark is desired. This is independent of the global system
   appearance.

### What NOT to do

- Do **not** store sidebar state or selected session in a global singleton.
- Do **not** share a `SessionViewModel` across windows — each window creates
  its own for its selected session.
- Do **not** use `@EnvironmentObject` for window-local state; use
  `@StateObject`.
- Do **not** use `@State` in the `App` struct for per-window state — `App`
  is a singleton; only scene-level and view-level state is per-window.

### Dock icon / AppKit activation

The current `AppDelegate` handles dock icon and activation policy. That's
global and correct — it runs once at launch, not per-window. Keep it as-is.

### Future: AppKit-owned windows

If the app later moves to AppKit-managed `NSWindowController` + `NSHostingView`
(instead of SwiftUI `WindowGroup`), the same rules apply: each `NSWindow`
owns its own `WindowViewModel`, and the hosting view is created fresh per
window. The `AppGlobalState` singleton is passed into each window's view
hierarchy.
