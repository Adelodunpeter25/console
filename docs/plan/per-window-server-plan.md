# Per-Window Server Selection Plan

## Goal

Let each desktop window connect to its own backend server (for example, Window 1
on local and Window 2 on staging) instead of all windows following one global
active-server pointer.

## Current behavior (as of 2026-09-10)

- The server list and `active_id` are stored as one global blob in `state.json`
  (`PersistedEnvironmentsState` in `apps/desktop/src/persistence/store.rs`).
- Every window boots from that same `active_id` with its own `ConsoleClient`
  (`state/app.rs`), and `init_environments` points the client at the active URL
  on startup (`state/environments.rs`).
- Switching servers in one window (`activate_environment`) resets that window's
  sessions/projects/tabs and reloads from the new backend, which is most of the
  per-window behavior already. It also rewrites the global `active_id`, so new
  windows follow the last switch while existing windows stay put. There is no
  way to express "this window is local, that window is staging" without them
  fighting over one pointer.
- Server management UI lives in Settings (`environment_rows`,
  `add/update/remove/activate_environment`); there is no in-window switcher.
- Each window currently reads `state.json` during construction. Therefore the
  persisted registry is global, but it is not currently a live shared registry:
  adding or editing a server in one window does not automatically update an
  already-open window.
- The workspace spec (`docs/plan/desktop-workspace-and-multiwindow-spec.md`)
  explicitly put full multi-window persistence out of scope; basic New Window
  (`Cmd+Shift+N`, `window.rs`) is session-only by design.

## Recommendation: global registry, session-only selection

Persist the environment registry globally. Keep the active environment in each
window's `ConsoleDesktopApp` and never persist ordinary switches. New windows
inherit the spawning window's environment. The persisted `active_id` is used
only as the startup default. Changes to the registry are broadcast to all open
windows, while registry edits never alter their current selections unless the
selected environment is removed.

This means:

- The environment list is one globally persisted registry, live-refreshed in
  all open windows through an explicit cross-window notification mechanism.
- `active_env_id` is window-local and session-only.
- A normal switch changes only the initiating window and does not write
  `active_id`.
- `active_id` is a cold-start default only. If retained for compatibility, it
  may be changed only by an explicit future "set as default" operation, never
  by ordinary per-window switching.
- No per-window-to-environment map is persisted across restarts.

## Implementation steps

1. `apps/desktop/src/window.rs`
   - Change `WindowLaunchTarget::Fresh` to carry an optional environment, for
     example `Fresh { environment_id: Option<String> }`.
   - The current focused window passes its `active_env_id` when creating a new
     window, so `Cmd+Shift+N` inherits the current selection.
   - Startup restore passes `None`; construction then loads and validates the
     persisted cold-start default.

2. `apps/desktop/src/state/app.rs` + `state/environments.rs`
   - Scope `active_env_id` to the window entity (`ConsoleDesktopApp`, which is
     already one instance per window).
   - Keep the existing switch reset semantics in `activate_environment`: clear
     sessions/projects/tabs/caches, re-point that window's `ConsoleClient` via
     `set_base_url`, and reload sessions/providers/projects/auth/usage. Remove
     only the global `active_id` write.
   - On startup, validate the persisted `active_id` against the loaded registry.
     If it was deleted or is otherwise invalid, select the first available
     environment, or the local default when the registry is empty, and keep the
     invalid ID from becoming the window's active selection.
   - Keep registry writes centralized so add/edit/remove persist the global
     list. Do not let editing an unrelated environment change any window's
     active selection. If the selected environment is removed, each affected
     window must select a valid fallback and perform the normal switch/reset
     flow.

3. Live registry propagation
   - Introduce an explicit cross-window registry-change notification, using the
     existing desktop window/application event mechanism rather than assuming
     that separately constructed windows share in-memory state.
   - After a successful add, edit, or remove, publish the updated registry (or
     an invalidation event that causes each window to reload it) to every open
     `ConsoleDesktopApp`.
   - Each window updates its environment list immediately. Registry updates
     must not change its current `active_env_id` unless that environment was
     removed or became invalid; in that case, choose a validated fallback and
     reconnect/reset only that window.
   - Ensure notification delivery also covers windows that are open but not
     currently focused, and avoid having each recipient write the same
     `active_id` back to disk.

4. In-window switcher (alongside the state change)
   - Add `Cmd+K` entries such as `Switch Server: <name>` backed by the active
     window's current environment list and its `activate_environment` method.
   - Check the existing command-palette architecture in
     `state/global_actions.rs` (`command_palette_entries`,
     `toggle_command_palette`) so entries are rebuilt from the active window's
     list rather than captured once from startup state.
   - Keep add/edit/remove in Settings for this phase. A footer/status dropdown
     can be added later, but switching must not require opening Settings.

5. `ConsoleClient` scoping
   - Audit `ConsoleClient::new(None)`, `set_base_url`, cloned client state, and
     all process-wide client/singleton call sites.
   - Add a focused test or equivalent verification proving that two separately
     constructed window clients can hold different base URLs and that changing
     one does not change the other. The test should cover the actual shared
     internal state boundary, not only call-site inspection.

6. Persistence
   - Do not add a per-window selection schema to `state.json`.
   - Continue persisting the global registry and retain `active_id` only as the
     cold-start default. Ordinary switching must never update it.
   - If an explicit "set as default" action is added later, document and test
     that it is the sole non-startup path allowed to update `active_id`.

## Verification

- Open Window 1 on local and Window 2 on staging; switching either window does
  not disturb the other's client, sessions, tabs, or active environment.
- Create a new window from a focused window and verify it inherits that
  window's environment. Verify cold start uses the persisted default.
- Delete or corrupt the persisted default and verify startup falls back to the
  first available environment or the local default without an invalid active
  ID.
- Add or edit an environment in Settings and verify every open window receives
  the updated list, including an unfocused window, without changing its active
  selection.
- Remove an environment and verify only windows using it select a valid
  fallback and perform the normal reset/reload; windows using another
  environment remain connected and selected as before.
- Verify the command palette rebuilds its switch entries from the active
  window's environment list and switches only that window.
- Verify switching still resets that window's tabs/caches and reloads from the
  new backend (the existing `activate_environment` semantics).
- Verify two independently constructed `ConsoleClient` instances retain
  isolated base URLs after either one switches.
- Run the relevant focused test file(s) for each touched area; fix and rerun
  until green before committing.

## Open questions for the user

1. Should per-window servers survive a restart (reopens the out-of-scope
   multi-window persistence decision), or stay session-only? Recommended:
   session-only.
2. New-window default: inherit the current window's server (recommended),
   always the global default, or a small picker at creation time?
3. Switcher UI: command-palette entries (recommended), footer/status dropdown,
   per-window Settings section, or a combination?
4. Editing semantics: add/edit/remove stays global, with only selection
   per-window and removal falling back only affected windows — any objections?
