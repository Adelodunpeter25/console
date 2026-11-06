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

## The workspace model, ported exactly

This is the phase where the preservation rule from [overview.md](overview.md) bites. The NSH
app reproduces the current model 1:1, quirks included:

- A workspace belongs to a project and holds that project's chats, browser tabs, terminals and
  file/diff tabs together. Tab kinds (`Chat`, `Browser`, `Terminal`, `FileAndDiff`), pane trees,
  drag/close paths (`workspace_panes.rs`, `console-ui::workspace::ops`) and stashed workspace
  trees per project (`project_workspace_roots`) all behave as today.
- Separate worktrees of the same project are separate workspaces, each with its own tabs and
  state, exactly as the GPUI app treats them today (`state/projects.rs`, `types/workspace.rs`).
- The persisted layout moves as it is: same file, same semantics, same migration of old files
  (or lack thereof). No "improvements" to defaults, ordering, or close behavior during the port.

## Approach

- Run the NSH app beside the GPUI app against the same server; feature-gap notes land in this
  file as they surface, not in a tracker elsewhere.
- Persistence: the store in `persistence/` moves into the core crate; TSX reads it via exports.

## Done when

- Daily use (chat, panes, palette, settings) is possible in the NSH app on desktop, with the
  workspace model indistinguishable from the GPUI app's — including its current bugs and UX.
- Headless tests cover each screen's core interactions and the workspace behaviors above.
