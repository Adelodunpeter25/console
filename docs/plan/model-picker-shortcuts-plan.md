# Model Picker Keyboard Shortcuts Plan

## Goal

- `Cmd+/` (Ctrl+/ on Linux/Windows) toggles the model picker for the active
  pane from anywhere in the window.
- Plain `/` moves focus into the picker search field, but only while the
  picker popover is open. Everywhere else `/` keeps its normal behavior
  (typing, composer slash commands).

## Current behavior (as of 2026-09-10)

- The picker (`ModelDropdownMenu` in
  `apps/desktop/crates/console-ui/src/common/model_picker.rs`) is a per-pane
  `popover` owned by `ComposerView` (`composer_view.rs`), toggled by clicking
  its trigger or pressing Enter/Space on the focused trigger.
- Each pane owns a `ContextMenuHandle` (`model_menu`) and a `ComposerInput`
  search entity (`model_search`). See `state/app.rs` (main pane) and
  `state/workspace_panes.rs` (`ensure_workspace_pane_state`, `pane_model_menu`,
  `pane_model_search`).
- The existing `on_toggle` observer already clears the search query and
  focuses the search field two frames after open (same cadence as the command
  palette, because popover content is deferred).
- No keyboard path to the picker exists. `/` currently has no binding.

## Proposed behavior

1. `Cmd+/` toggles the active pane's picker:
   - Closed -> open via the existing `toggle_popover` path, reusing the
     clear + autofocus observer. No new focus code on the open path.
   - Open -> close and return focus to that pane's composer input so the
     keyboard flow feels complete.
2. `/` is a no-op for the picker unless the picker popover card is open:
   - Picker open + focus elsewhere in the card -> focus the pane's
     `pane_model_search` input.
   - Search already focused -> propagate so `/` inserts text (searching
     `and/or` etc. must keep working).
   - Picker closed -> `/` types/fires normally in the composer and
     everywhere else.
3. `Esc` keeps dismissing the picker (existing `MENU_CONTEXT` behavior).

## Implementation steps

1. `apps/desktop/src/keybindings.rs`
   - Add `ToggleModelPicker` and `FocusModelSearch` actions to the
     `console_global` block.
   - Bind `secondary-/` globally (no key context) so it fires from the
     composer, transcript, sidebar, and terminal.
   - Bind `/` scoped to the menu key context (`ConsoleMenu`, the context the
     popover card already sets) so it can only fire while a menu/popover is
     open. Verify GPUI's exact keystroke spelling for `/` vs `slash` on macOS
     before finalizing.
   - Default: `Cmd+/` = toggle (not open-only).
2. `apps/desktop/src/state/global_actions.rs`
   - Add `toggle_model_picker(window, cx)` next to `focus_composer`:
     - Route through `get_active_window` -> `active_pane_id` ->
       `pane_model_menu()`.
     - Toggle via the existing `toggle_popover(handle, ...)` helper (the
       documented "as if its trigger were clicked" path), deferred with
       `cx.defer` + `window.update` like the `Cmd+K` handler, since toggle
       observers mutate the owning entity.
     - Guard like `focus_composer`: if the command / quick-open /
       project-browse palette is open, yield to it.
   - Add `focus_model_search(window, cx)`:
     - If the active pane's picker is open but the search field is not
       focused, focus `pane_model_search`.
     - If search is already focused, propagate so `/` inserts text (mirrors
       the existing `Cmd+C` -> `CopySelection` propagate precedent).
3. `apps/desktop/src/state/workspace_panes.rs` + `state/app.rs`
   - Fix the "still open?" check inside the autofocus callback: it currently
     reads the `pane-main` compat handle instead of the pane's own menu.
     Per-pane toggle needs that check to query the pane's handle so splits
     don't cross-focus.
4. `apps/desktop/crates/console-ui/src/primitives/menu.rs`
   - The menu context string (`MENU_CONTEXT = "ConsoleMenu"`) is currently
     private; export it (or a public const) so the `/` binding can reference
     it without duplicating the literal.
5. Wire handlers in `keybindings::init_handlers` following the existing
   `ToggleCommandPalette` / `FocusComposer` pattern (active-window routing).

## Verification

- `Cmd+/` opens the picker from the composer, sidebar, and transcript.
- `Cmd+/` again closes it and returns focus to the composer's pane.
- `/` with picker open but unfocused moves focus to search; `/` in search
  types a literal slash; `/` in the composer is untouched.
- `Esc` still dismisses; per-pane pickers in splits don't cross-focus.
- Run the relevant test file(s) for touched areas (never the full suite
  unless asked); fix and re-run until green before committing.

## Open decisions

- Default is toggle for `Cmd+/`; confirm open-only is not preferred.
- Confirm `/` is refocus-only and must never open the picker.
