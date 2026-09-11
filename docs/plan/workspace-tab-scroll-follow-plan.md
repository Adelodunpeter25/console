# Workspace tab scroll-follow plan (desktop)

## Goal

When the active tab in a workspace pane is outside the visible tab strip
(easy with 4+ chats + a terminal in one pane), the strip auto-scrolls the
minimum distance to bring it fully into view — like VS Code / Chrome. Covers
every activation path (click, Option+1–9, sidebar, palette, drag-drop,
session restore) plus window/split resizes that push the active tab out of
view without any tab change.

## Current behavior (from code review)

- The strip is a plain `div` with `.overflow_x_scroll()` and **no
  `ScrollHandle`** (`crates/console-ui/src/workspace/tab_bar.rs:84-91`), so
  today the offset is user-driven only (wheel/drag) and nothing can read or
  move it programmatically. A newly activated off-screen tab just sits there.
- Each tab already has a stable id (`ElementId::Name(tab_id)`,
  `tab_bar.rs:105`) — the hook a follow implementation measures against.
- `WorkspaceTabBar` is `RenderOnce`, rebuilt every frame in
  `workspace/pane.rs::render_leaf` (line 199) from a `LeafPaneNode` snapshot;
  it has no retained state and no access to app state in `render`.
- Per-pane retained state exists: `WorkspacePaneState`
  (`src/types/workspace.rs`), keyed by pane id in `workspace_pane_states`
  with clear create/remove lifecycle (`state/workspace_panes.rs:16`,
  `state/workspace_panes.rs:1350`).
- Reusable scroll-follow precedent: `single_line_scroll` minimum-move math +
  `follow_caret` (`ScrollHandle::bounds/offset/set_offset` +
  `window.request_animation_frame()` for the next-frame correction) in
  `crates/console-ui/src/common/input/element.rs:82-137`.

## Design

Render-time gating keyed off `active_tab_id` (path-agnostic: every activation
path already funnels through `select_workspace_tab` / `activate_workspace_tab`
in `state/workspace_panes.rs`, and render sees the result regardless).

1. **Per-pane follow state.** New struct (e.g. `TabStripFollow`) holding a
   `ScrollHandle` plus `last_tab: Option<String>` and last viewport
   origin/width, behind `Rc<RefCell<…>>` (paint callbacks are `'static` +
   `FnMut`, `render` only gets `&mut App`). Lives in `WorkspacePaneState`
   (created in `ensure_workspace_pane_state`, dies with the pane) so split
   panes each track their own strip.
2. **Plumb into the tab bar.** App render path passes the pane's follow state
   through the workspace-pane render into `render_leaf`
   (`workspace/pane.rs:173`) → `WorkspaceTabBar` (new builder method like
   `with_new_tab`, e.g. `with_scroll_follow`). The `workspace-tabs-scroll`
   div gets `.track_scroll(&handle)`.
3. **Measure with a probe, not layout ids.** Add an invisible full-size
   `canvas` as the first child of the *active* tab only (`absolute`,
   `inset_0`, paints nothing — same zero-cost overlay pattern as
   `selection_input` in `common/error_banner.rs:82` and `frame_reset` in
   `markdown/render.rs:783`; tab div needs `.relative()`). In its paint
   callback its `bounds` are the active tab's rect in window coordinates;
   compare against `scroll_handle.bounds()` (the visible strip):
   - Gate: skip unless `active_tab_id != last_tab` (activation changed) or
     the viewport origin/width changed since last follow (resize/split
     case). This is what stops the follow from fighting the user's manual
     wheel-scrolls — VS Code behaves the same (follow on selection change,
     never yank-back). Then record the new generation.
   - Math: minimum-move correction, horizontal analogue of
     `single_line_scroll` (`element.rs:82`): if tab left is left of the
     viewport, shift left by the difference; else if tab right is right of
     the viewport, shift right; clamp to `[0, max_offset]`; ignore moves
     under ~0.5px. `set_offset` + `window.request_animation_frame()` exactly
     like `follow_caret`, since the container consumed this frame's offset
     before painting children.
4. **What NOT to do:** no smooth-scroll animation (`set_offset` jumps, same
   as native tab bars); no following in unfocused panes specially — every
   pane follows its own strip, which is harmless and simpler; no tab-width
   estimation (the probe measures real laid-out rects, so truncation at
   `max_w(140)` and icon/padding differences are handled for free).

## Edge cases

- **Resize/split:** viewport origin+width in the gate re-triggers follow with
  no tab change.
- **Tab close:** active id changes to the neighbor → follow runs; GPUI clamps
  any over-max offset.
- **New tab at end:** becomes active → `active_tab_id` changed → strip
  scrolls to reveal it.
- **Tab wider than viewport** (tiny splits): minimum-move keeps the left edge
  visible, matching `single_line_scroll` head-tracking.
- **First mount / session restore:** `last_tab` starts `None`, so the first
  paint follows automatically.

## Verification

- Manual: open 5+ sessions + terminal in one pane, shrink the window until
  tabs overflow; click a hidden tab from the sidebar, Option+1–9 to a hidden
  tab, drag a tab in from another pane, close the visible tabs — the strip
  brings the active tab into view each time with no jitter on manual
  wheel-scroll.
- `cargo check` for the desktop workspace when the tree is free (another
  agent is active — do not run it concurrently).
