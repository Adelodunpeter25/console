# Phase 2 — workspace screens: chat, tabs, settings

**Goal**: the screens people live in, in TSX. Order within the phase is by iteration cost:
each screen is hot-reloadable and independently testable.

## Screens

- **Chat / transcript** — the phase-0 spike screen, productionized against real sessions
  (streaming, markdown, code blocks, composer with attachments).
- **Workspace panes and tabs** — pane splitting, tab strip (browser/terminal/file-diff tabs are
  stubs until phases 3–4); the layout model moves into the Rust core so panes persist.
- **Command palette** — `nsh/base` `Command`, bound with `keymap.bind("mod+k", …)`,
  `NotEditing` semantics so it doesn't fight text fields.
- **Settings** — dialogs/pages driven by the Rust settings state; replaces
  `settings_window.rs` + `settings/`.
- **Keybindings** — `apps/desktop/src/keybindings.rs` becomes data in the core, rendered through
  `keymap` and shown in the palette with `key_label()`.

## Approach

- Run the NSH app beside the GPUI app against the same server; feature-gap notes land in this
  file as they surface, not in a tracker elsewhere.
- Persistence: the store in `persistence/` moves into the core crate; TSX reads it via exports.

## Done when

- Daily use (chat, panes, palette, settings) is possible in the NSH app on desktop.
- Headless tests cover each screen's core interactions.
