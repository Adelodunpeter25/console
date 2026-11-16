# Remove the right inspector panel, add a tab-bar dropdown

Delete the right-hand inspector column (file tree, changes list, browser,
devices, terminal bottom split) and replace the workspace tab bar's `+` button
with an Amp-style dropdown. The workspace layout stays exactly as it is: chat
panes, splits, and tabs all keep working the same way; only the third column
goes away and the `+` becomes a menu.

Design references: the Ampcode screenshot (right panel = one view at a time,
dropdown switcher) and the two `2026-11-16 11.36` screenshots showing the
redesigned Changes review tab.

## Status

All six slices are implemented and committed. Nothing has been exercised in the
running app yet; `cargo check --workspace --tests` is clean and
`git_badges_test` passes.

| # | Slice | State | Commit |
|---|---|---|---|
| 1 | FileTree tab | Done | `2fbcdc16` |
| 2 | Device tab | Done | `26f974c0` |
| 3 | ChangesReview redesign | Done, partial (see below) | `a5ec284a` |
| 4 | Browser retarget | Done | `ab25ad9b` |
| 5 | `+` dropdown | Done | `9188203c` |
| 6 | Delete the panel | Done | `a77a1fa8`, `e49ff3ae` |

Also done, beyond the original slices:

- ⌘K command palette: "Toggle Inspector" replaced by **Open Files**, **Open
  Changes** and (macOS) **Open Devices**.
- Title-bar inspector toggle, `alt-b`, the `ToggleRightSidebar` action, its
  app-menu item and its keybindings-page row are gone.
- Files, Changes, Devices, Terminal and Browser from the `+` menu and the
  palette open in the right-hand split beside the chat (the same pane
  `target_subtab_pane_id` picks for new browser tabs). New Chat still opens the
  floating composer.

### Not done

- **ChangesReview redesign is partial.** Rows now show filename and folder, `+N`
  `-N`, an A/M/D badge and the existing Mark as reviewed button, with
  full-bleed diffs. Not built from the reference screenshot: the commit header
  bar (branch and ahead/behind), "N unmodified lines" expanders, and the "…"
  row menu. The expanders need support in the diff viewer.
- **Run panel and script logs have no UI.** Their only surface was the bottom
  split. The state (`project_scripts.rs`, script-log and terminal state, `run/`
  components) is intentionally kept for a future home, so the compiler warns
  about unused code until then.
- **Manual verification** from section 9 is still to do: every menu item, the
  split placement, device/browser surfaces showing only after canvas sync, an
  old `layout.json` loading, and an agent browser action hitting the right tab.
- **Not implemented from the plan text:** the Files tab has no header row or
  search box (just the tree); `open_auxiliary_tabs` entries are dropped on load
  without an explicit migration (the field is simply no longer read).

## 1. What gets deleted, what stays

### Deleted (the whole column)

- `console_ui::inspector::RightSidebar` — the shell, tab strip (All files /
  Changes / Browser / Devices), add-tab menu, and resize handles
  (`crates/console-ui/src/inspector/right_sidebar.rs`).
- `console_ui::inspector::RightSidebarBottomSplit` — the terminal + run panel
  split under the inspector (`inspector/bottom_split.rs`).
- Inspector app state: `right_sidebar_visible`, `right_sidebar_width`,
  `right_sidebar_bottom_height`, `right_sidebar_bottom_collapsed`,
  `inspector_active_tab`, `inspector_open_auxiliary_tabs`,
  `inspector_browser_tab_id`, `sidebar_session_browsers`, `inspector_tree`,
  `inspector_fs_watch`, `inspector_git_watch`, the resize/bookkeeping methods
  in `src/state/right_sidebar.rs`, and the third column in `src/view/mod.rs`.
- `PrimaryTab` / `AuxiliaryTab` / `InspectorTab` / `InspectorWatches` types.

### Stays untouched

- The workspace tree: panes, splits, tab strip, drag & drop.
- Terminals as workspace tabs (the inspector's duplicate terminal split just
  goes away).
- Browser as a workspace tab. The inspector's chat-scoped browser variant is
  removed; agent browser actions retarget workspace browser tabs (slice 4).
- Subagents / Shells popovers on the chat pill row (already moved there).
- `src/state/project_scripts.rs` and all script-log state. The Run panel UI
  disappears with the column (its only surface is the bottom split), but the
  state is left in place so it can be re-homed later; no Scripts tab is built
  in this plan.

## 2. New homes for inspector capabilities that have no workspace equivalent

| Inspector capability | New home |
|---|---|
| Files tree | New `WorkspaceTabConfig::FileTree` tab, rendering the existing `console_ui::FileTreeView` |
| Devices preview | New `WorkspaceTabConfig::Device` tab (macOS-only native surface, like the browser) |
| Changes list | The existing `ChangesReview` tab, redesigned per the screenshots (section 5) |

### FileTree tab

- Add the variant to `WorkspaceTabConfig` (`crates/console-core/src/types/
  workspace.rs`) with `project_id`, `title`, `last_active_at_ms`, following the
  `ChangesReview` pattern: serde tags, `id()` (`files`), `title()`,
  `project_id()` arms, and `set_last_active_at_ms`.
- Render in `render_workspace_content` (`src/view/workspace_content.rs`):
  header row + `FileTreeView` built from the tree that the inspector fed from
  `inspector_tree`. Per-pane/per-tab tree state replaces the single global
  `inspector_tree` (a `HashMap<tab_id, Rc<Vec<FileTreeNode>>>` mirroring
  `changes_review_data`).
- Persistence: the tab round-trips through the existing workspace
  persistence like other tab kinds; no tree contents are saved.

### Device tab

- Same variant pattern (`id()` = `device`), macOS-gated like the inspector's
  Devices tab: on Windows the dropdown omits the entry (`AuxiliaryTab::ALL`
  already does this today).
- The native device view (`device_view_for_inspector`) moves its host to the
  tab's content area. It follows the webview rules we already fixed for the
  browser: shown only after its canvas syncs bounds, hidden by every other
  path, `cx.notify()` on visibility changes.

## 3. The `+` dropdown

`WorkspaceTabBar` (`crates/console-ui/src/workspace/tab_bar.rs`) currently
calls `with_new_tab`, which opens the floating composer
(`src/view/mod.rs:1069`). Replace the `+` button with the existing
`dropdown_menu` primitive (`crates/console-ui/src/primitives/menu.rs`):

- `MenuItem::new` rows with `icon(...)` and `shortcut(...)`:
  - **New Chat** — same action as today's `+` (`open_floating_composer`)
  - **Files** — opens a `FileTree` tab in the right-hand split
  - **Changes** — opens the redesigned `ChangesReview` tab for the focused
    chat (see section 5), in the right-hand split
  - **Terminal** — opens a terminal tab in the right-hand split
  - **Browser** — opens a browser tab in the right-hand split
  - **Devices** — opens a `Device` tab in the right-hand split (macOS only)
- `+` is fully replaced; no chevron beside it.
- Everything except New Chat opens in the right-hand split beside the chat
  (`target_subtab_pane_id`: reuse the other pane, or split the chat's pane),
  not the clicked pane. This replaced the original "active pane" rule.
- Each open goes through the same `sync_workspace_webviews` treatment chat /
  file / terminal / browser opens already use.

### Shortcuts

Decided: no new shortcuts. The proposed `ctrl-*` set was built and then removed.
The menu rows only show the keys that already exist: New Chat `⌘N`, Terminal
`⇧⌘T`, Browser `⇧⌘B`. Files, Changes and Devices have no shortcut.

## 4. Watch policies move with the views

`InspectorTab::watches()` decides when the fs and git streams run
(`src/state/right_sidebar.rs:637` `inspector_tree_visible`). The rule becomes
tab-visibility based:

- fs watch runs while a `FileTree` tab is the active tab of any visible pane.
- git watch runs while a `FileTree` or `ChangesReview` tab is active (it
  feeds status badges and working changes).
- The watcher lifecycle code (`ensure_inspector_fs_watcher` and the git
  equivalent) moves into the workspace-tab state module with the same
  one-stream-per-target reuse discipline.

## 5. ChangesReview tab redesign

The screenshots show the new review layout the tab should match. The data
model is unchanged (`SessionFileChange`, per-`(path, turn_index)` reviewed
flags, `changes_review_data` / `changes_review_collapsed` state, the existing
fetch and toggle endpoints in `src/state/workspace_tabs/changes_review.rs`).
What changes is the rendering (`src/view/workspace_content.rs`) to the design
in the screenshots: file blocks with status/review affordances styled like the
reference, diffs opening from the rows into the existing diff viewer, and
reviewed-state toggles keeping the optimistic-then-confirmed flow. The exact
visual spec is taken from the screenshots at implementation time.

## 6. Browser action retargeting

`src/state/browser_actions.rs` scopes agent-driven browser actions to the
inspector's chat browser (`sidebar_session_browsers`,
`inspector_browser_tab_id`). After the removal those actions must resolve to
the chat workspace's browser tabs only (the paths at lines 904–935 already
walk `workspace_root` for part of this). No behavior change for the user: a
chat with a browser tab in its workspace keeps receiving agent navigation;
without one, actions no-op as today.

## 7. Persistence

- `PersistedLayoutState` (`src/persistence/layout.rs`): drop the
  `right_sidebar_*` fields, keeping serde defaults so existing `layout.json`
  files still deserialize; stale keys are ignored on load.
- Migrate `open_auxiliary_tabs` entries out (they referenced inspector
  browser tabs); the inspector's open tabs are simply not restored.
- New tab kinds persist through the existing workspace-state machinery; check
  `src/persistence/workspace_state.rs` for kind-specific handling of
  `Browser` and mirror it for `FileTree` / `Device`.

## 8. Slices (each independently shippable, one commit each)

Status of each is in the table at the top; slice 6 landed as two commits.

1. **FileTree tab** — variant, render, tree state, watch-policy move. Nothing
   is removed yet; the inspector keeps working beside it.
2. **Device tab** — variant, render, native-surface show/hide following the
   canvas-sync rules.
3. **ChangesReview redesign** — new rendering per the screenshots; state and
   endpoints unchanged.
4. **Browser retarget** — agent browser actions resolve workspace browser
   tabs; the inspector browser keeps working in parallel.
5. **The dropdown** — `+` becomes the menu; menu items open the tabs built in
   slices 1–2 plus chat/terminal/browser; keybindings added.
6. **Delete the panel** — remove the column, the shells, the inspector state
   and types, the third-column render block in `src/view/mod.rs`, and the
   persistence fields.

Order matters: nothing user-visible disappears until slice 6, and slice 6 is
mechanical once 1–5 land.

## 9. Verification

After every slice:

- `cd apps/desktop && cargo check -p console-app`
- Drive the app: open each menu item, confirm the tab lands in the active
  pane; switch workspaces and confirm the device/browser native surfaces
  appear only after their canvas syncs (no 800×600 flash at window origin);
  open a saved pre-change `layout.json` and confirm it loads without the
  third column; trigger an agent browser action with a workspace browser open
  and confirm it navigates the right tab.

## 10. Open decisions (flagged, not blocking)

- **Run panel / script logs**: decided: removed from the UI, state and
  components kept, no stopgap. A new home is still open.
- **Shortcut keys**: decided: none added (see section 3).
