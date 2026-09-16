# Browser Workspace Tab Plan

## Goal

Add Browser as a normal workspace tab (Chat / Terminal / File / Diff / Browser) while keeping the existing inspector Browser tab untouched for now.

Port-open and chat-link clicks get smart reuse: focus the tab if the URL is already open, otherwise overwrite the active browser tab when sensible, else open a new tab. Scope tabs by `project_id`, matching every other tab type. No `workspaceId` concept is introduced.

## Current state

- `WorkspaceTabConfig` (`apps/desktop/crates/console-core/src/types/workspace.rs:14`) has Chat / Terminal / File / Diff, each with `project_id: Option<String>` plus `id()` / `title()` / `project_id()` helpers.
- Content dispatch is `render_workspace_content` (`apps/desktop/src/view/workspace_content.rs:20`) with early returns for Terminal / File / Diff, chat as fallback.
- Browser today is a singleton inspector view: `browser_view: Option<Entity<BrowserView>>` (`apps/desktop/src/state/app.rs:222`), created in `browser_view_for_inspector` (`apps/desktop/src/state/right_sidebar.rs:333`), driven by `open_port_in_browser` (`apps/desktop/src/state/port_forward.rs:161`), which forces the right sidebar + `AuxiliaryTab::Browser` visible.
- Terminals already solve multi-instance: `terminals: HashMap<id, Entity<TerminalView>>` + `dispose_closed_tab` kills the PTY (`apps/desktop/src/state/workspace_panes.rs:1138`). Browser needs the same treatment for `BrowserView` (native WKWebView host, `close()` + `sync_native_state`).
- Persistence is `workspace.json` version 1 (`apps/desktop/src/persistence/workspace.rs:8`) with `#[serde(tag = "type")]`. A new variant breaks old docs without a version bump / tolerant load.
- Inspector target resolution (`active_inspector_target`, `apps/desktop/src/state/right_sidebar.rs:211`) already scopes File / Diff / Terminal by `project_id` with pane-project fallback. Browser tabs follow the same rule.

## Design decisions

- **Keep inspector browser.** Workspace tabs and inspector browser coexist. Workspace `BrowserView` entities are separate from `self.browser_view`. No shared singleton.
- **Scope by `project_id`.** Same as File / Terminal / Diff: store `project_id: Option<String>` on the tab, default from `pane_project_id()`, fall back to pane project for cwd-dependent behavior. URLs themselves stay global (localhost forwards are backend-global, see `forwarded_ports_by_project["global"]`).
- **One `BrowserView` entity per tab**, keyed by `browser_id`. Only the active tab's webview is visible / natively focused; background tabs call `sync_native_state(false, ...)`. Closing a tab calls `view.close()` and drops the entity (mirrors terminal PTY kill).
- **Smart reuse on port-open / chat-link open** (the "advanced" behavior):
  1. Normalize the requested URL (strip trailing `/`, default scheme).
  2. If any workspace browser tab already has that URL → focus its pane + tab, navigate only if drifted.
  3. Else if the active tab is a browser tab opened from a port/link (transient, unpinned, no manual navigation since open) → navigate it in place (overwrite).
  4. Else → open a new browser tab in the active pane.
  5. Chat-link clicks (`open_chat_url_in_browser` → `open_port_in_browser` path) use the same function.

## Implementation steps

### 1. Core type: `WorkspaceTabConfig::Browser`

File: `apps/desktop/crates/console-core/src/types/workspace.rs`

- Add variant: `Browser { browser_id: String, url: String, title: String, project_id: Option<String>, last_active_at_ms: Option<i64> }` with `#[serde(rename = "browser")]`.
- Extend `id()` → `browser:{browser_id}`, `title()`, `set_title()`, `project_id()`, `set_project_id()`, `last_active_at_ms()` / setter.
- URL is persisted verbatim; title defaults to host + port (e.g. `localhost:3000`) at open time and updates from the live page title (`BrowserView::title_changed`) when available.
- Favicon + title come from the current page URL: `url_host()`, `favicon_url()` (Google favicon service, `https://www.google.com/s2/favicons?domain={host}&sz={size}`), and `default_browser_title()` in `crates/console-ui/src/browser/address.rs`. No network in the helpers — views use the URL directly in `img()`. `BrowserView::favicon_url(size)` exposes it per surface.

### 2. Tab ops + persistence migration

Files: `apps/desktop/crates/console-ui/src/workspace/ops.rs`, `apps/desktop/src/persistence/workspace.rs`, `workspace_state.rs`

- Verify generic ops (`open_tab`, `select_tab`, `close_tab`, `close_matching_tabs`, `find_tab`, drag) work with the new variant — they match on `id()`, so no logic change expected; add coverage for `browser:` ids.
- Bump `WORKSPACES_VERSION` 1 → 2. Old docs (v1) load as-is since serde ignores unknown variants only when the tag is unknown on read — actually a v2 doc on an old binary fails closed, which is correct. On read, unknown `browser` tabs from the future must not wipe the tree: keep the existing whole-doc fallback, document it.
- Stashed `project_workspace_roots` eviction in `close_workspace_tab` already closes by `tab.id()` — browser ids flow through automatically.

### 3. Per-tab browser state + open / reuse actions

File: `apps/desktop/src/state/` (new `browser_tabs.rs`, wired in `state/mod.rs` + `state/app.rs`)

- `browser_views: HashMap<String, Entity<BrowserView>>` on `ConsoleDesktopApp` (browser_id → view), parallel to `terminals`.
- `open_browser_tab(url, title?, pane_id?)` — creates `browser_id = browser-{ms}-{n}`, stamps `project_id = pane_project_id()`, opens `WorkspaceTabConfig::Browser`, persists, notifies.
- `open_browser_smart(url, window, cx)` — the reuse entry point used by port-open and chat links:
  - normalize URL;
  - scan all leaves for `Browser` tabs with matching normalized URL → `select_tab` + focus pane;
  - else if active tab is `Browser` and `is_transient_browser_tab()` (opened from port/link, address bar untouched since open) → `navigate_to_url` in place;
  - else `open_browser_tab` in active pane.
- Track transience minimally: `transient_browser_tabs: HashSet<browser_id>` set on smart-open, cleared on manual address submit (hook the address `ComposerEvent::Submit` via a subscription or a `mark_browser_navigated(browser_id)` call from the view wrapper).
- `dispose_closed_browser_tab(tab, cx)`: if no remaining tab references `browser_id`, `view.close(cx)` + remove from map + clear transience flag. Extend `dispose_closed_tab` match (currently Terminal / File|Diff / Chat) with a Browser arm.

### 4. Render branch

File: `apps/desktop/src/view/workspace_content.rs`

- Before the chat fallback, add: `if let Some(WorkspaceTabConfig::Browser { browser_id, url, .. })` → get-or-create `BrowserView` for `browser_id` (seed with `url` on first create via `navigate_to_url` after mount), return it full-size.
- Non-active browser tabs are not rendered (existing per-pane active-tab model already guarantees this), but on tab switch call `sync_native_state(visible)` so the outgoing webview hides and releases focus, mirroring `sync_inspector_webviews`.
- No cwd/project gate: unlike Terminal (which requires a project for cwd), browser renders with just the URL. Missing project only affects inspector-target fallback, not rendering.

### 5. Reroute port-open + chat links

Files: `apps/desktop/src/state/port_forward.rs`, `apps/desktop/src/state/workspace_panes.rs`, `apps/desktop/src/state/transcript_wiring.rs`

- `open_port_in_browser(url, window, cx)` → calls `open_browser_smart(url, ...)` (workspace tab). Keep inspector behavior? No — per this plan the workspace tab becomes the default destination; inspector browser stays available manually but is no longer driven by port-open.
- `open_chat_url_in_browser` already delegates to `open_port_in_browser`, so it inherits reuse for free.
- `active_inspector_target` match gains a `Browser { project_id, .. }` arm identical to File / Diff / Terminal (resolve cwd from `project_id` → pane project). This keeps inspector files/changes/terminals in sync when a browser tab is active.

### 6. Tab strip + inputs + tab palette + shortcuts

Files: `apps/desktop/crates/console-ui/src/workspace/tab_bar.rs`, `apps/desktop/src/keybindings.rs`, `apps/desktop/src/state/global_actions.rs`

- Browser tab icon (globe), title = host or page title, close button + drag reuse existing machinery.
- `Cmd+T` stays the tab palette (`toggle_tab_palette`): Browsers first (by recency), then Terminals, then Chats (by recency). No dedicated new-browser-tab shortcut — creation lives in the `Cmd+K` palette as "New Browser Tab".
- Tab palette (`toggle_tab_palette`) lists browser tabs with globe icons: Browsers first (by recency), then Terminals, then Chats (by recency). Search matches workspace tab titles via the palette label. File / Diff tabs stay out of scope.
- Address bar lives inside `BrowserView` already — no new input needed.

## Verification

- `cargo build --manifest-path apps/desktop/Cargo.toml` (or `make desktop-check` if available).
- Manual: open port from Run panel → new browser tab; click same port again → focuses existing tab, no duplicate; click different port with a transient browser tab active → overwrites in place; manually navigated tab is never overwritten.
- Manual: split panes with two browser tabs, switch tabs, confirm only the active webview receives input and background audio/video suspends (hide behavior).
- Manual: close browser tab → native view destroyed (no leaked WKWebView process growth); restart app → browser tabs restore with URL + title.
- Existing workspace persistence tests / tab ops tests still pass; add one unit test for URL normalization + reuse selection if a natural home exists (no new harness).

## Out of scope

- Removing the inspector Browser tab.
- Multi-window sync of browser tabs.
- Preview-server discovery, downloads, popups, DevTools entitlements.
- Pinning UI for browser tabs (transience is implicit; pinning can come later).
