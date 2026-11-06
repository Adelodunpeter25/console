# Phase 2 — workspace screens: chat, tabs, settings

**Goal**: the screens people live in, in TSX. Order within the phase is by iteration cost:
each screen is hot-reloadable and independently testable.

## Subphases

One screen (or component family) at a time, in dependency order, mirroring how the GPUI app
separates its components (`console-ui/src/…`). Each subphase lands alone: hot-reloadable,
independently testable, and usable in the shell when done. The NSH app's `src/` tree grows the
same folders, so like lives next to like.

- **2.1 App shell and layout** (`layout/`, `layout/sidebar/`) — title bar, window chrome, the
  project/session sidebar with groups, drafts and loading states. The nav placeholder from
  phase 1 becomes this.
- **2.2 Primitives** (`primitives/`, `common/` small pieces) — context menu, menu, tooltip,
  scrollbar, text field, icons and file icons, motion, palettes and picker menus. Everything
  later subphases compose; ported just ahead of the first screen that needs each piece.
- **2.3 Workspace panes and tabs** (`workspace/`, `state/workspace_panes.rs`,
  `state/workspace_tabs/`, `persistence/`) — pane tree, tab strip, split/drag/close, empty
  state; the persisted layout moves into the core crate. Tab kinds other than chat are stubs
  until 2.5–2.7 and phase 3.
- **2.4 Chat tab** (`chat/`, `common/composer_view.rs`, `common/input/`, `markdown/`) — the
  phase-0 spike productionized: transcript, message bubbles, composer with attachments and
  autocomplete, tool calls, thinking blocks, streaming markdown with highlighting, the visual
  components (table, chart, metric, select).
- **2.5 Viewer tabs** (`viewer/`) — file, diff and code viewers, changes review, image preview,
  logs, file comments, blocked panel.
- **2.6 Inspector, right sidebar and run** (`inspector/`, `run/`) — bottom split, changes list,
  file tree, right sidebar, run panel and script log.
- **2.7 Terminal tab** (`terminal/`) — the terminal view over a PTY from the core; the heaviest
  single tab, so it follows the viewers.
- **2.8 Settings and onboarding** (`settings/`, `onboarding/`, `state/settings.rs`) — pages and
  modals driven by the Rust settings state; keybindings become data in the core, rendered
  through `keymap` and shown in the palette.
- **2.9 Command palette and global search** (`common/global_search_panel.rs`,
  `common/quick_open_palette.rs`) — `mod+k` palette with `NotEditing` semantics, global search,
  project browse.

Browser and device tabs stay out of phase 2 — they are phase 3
([phase-3-browser-and-devices.md](phase-3-browser-and-devices.md)).

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
app reproduces the current model 1:1, with the same functionality and UX:

- A workspace belongs to a project and holds that project's chats, browser tabs, terminals and
  file/diff tabs together. Tab kinds (`Chat`, `Browser`, `Terminal`, `FileAndDiff`), pane trees,
  drag/close paths (`workspace_panes.rs`, `console-ui::workspace::ops`) and stashed workspace
  trees per project (`project_workspace_roots`) all behave as today.
- Separate worktrees of the same project are separate workspaces, each with its own tabs and
  state, exactly as the GPUI app treats them today (`state/projects.rs`, `types/workspace.rs`).
- The persisted layout moves as it is: same file, same semantics, same migration of old files
  (or lack thereof). Defaults, ordering and close behavior stay as users know them.
- Known bugs and UX papercuts in this surface are fixed here deliberately, each noted — not
  silently changed and not carried over.

## Approach

- Run the NSH app beside the GPUI app against the same server; feature-gap notes land in this
  file as they surface, not in a tracker elsewhere.
- Persistence: the store in `persistence/` moves into the core crate; TSX reads it via exports.

## Done when

- Daily use (chat, panes, palette, settings) is possible in the NSH app on desktop, with every
  functionality and the UX of the GPUI app intact, minus its known bugs.
- Headless tests cover each screen's core interactions and the workspace behaviors above.
