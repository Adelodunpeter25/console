# Per-Window Server Selection Plan

## Goal

Let each desktop window connect to its own backend server (e.g. Window 1 on
local, Window 2 on staging) instead of all windows following one global
active server pointer.

## Current behavior (as of 2026-09-10)

- The server list + `active_id` is one global blob in `state.json`
  (`PersistedEnvironmentsState` in `apps/desktop/src/persistence/store.rs`).
- Every window boots from that same `active_id` with its own `ConsoleClient`
  (`state/app.rs`), and `init_environments` points the client at the active
  URL on startup (`state/environments.rs`).
- Switching servers in one window (`activate_environment`) resets that
  window's sessions/projects/tabs and reloads from the new backend — which is
  most of the per-window behavior already. But it also rewrites the global
  `active_id`, so new windows follow the last switch while existing windows
  stay put. There is no way to express "this window is local, that window is
  staging" without them fighting over one pointer.
- Server management UI lives in Settings (`environment_rows`,
  `add/update/remove/activate_environment`); there is no in-window switcher.
- The workspace spec (`docs/plan/desktop-workspace-and-multiwindow-spec.md`)
  explicitly put full multi-window persistence out of scope; basic New Window
  (`Cmd+Shift+N`, `window.rs`) is session-only by design.

## Recommendation: session-only independence

- Keep the server *list* global and persisted (one place to add/edit/remove;
  all windows see new entries immediately).
- Make the *active selection* per-window and session-only:
  - New windows inherit the spawning window's server (or the global default
    on cold start).
  - Switching in one window never changes another window's connection and
    never rewrites the global default out from under other windows.
- This keeps the "multi-window persistence out of scope" decision intact: no
  per-window->server map is persisted across restarts.

## Implementation steps

1. `apps/desktop/src/window.rs`
   - Extend `WindowLaunchTarget::Fresh` to carry an optional environment
     (inherit vs default), so `Cmd+Shift+N` inherits the active window's
     server by default. `RestorePersisted` keeps using the stored global
     default.
2. `apps/desktop/src/state/app.rs` + `state/environments.rs`
   - Scope `active_env_id` to the window entity (per-`ConsoleDesktopApp`
     instance, which is already per-window).
   - Keep `save_persisted_environments` writing the server list, but stop
     persisting the active pointer on every switch; persist it only (if at
     all) as a "last global default" used for cold start.
   - Keep the existing switch reset semantics in `activate_environment`
     (clear sessions/projects/tabs/caches, re-point `ConsoleClient` via
     `set_base_url`, reload sessions/providers/projects/auth/usage) — just
     without the global side effect.
3. Switcher surface (recommended: command palette + keep Settings as editor)
   - Add `Cmd+K` entries like "Switch Server: <name>" backed by the shared
     environment list and the active window's `activate_environment`.
   - Optionally add a lightweight footer/status dropdown later; do NOT move
     add/edit/remove out of Settings in this phase.
   - Follow the existing palette patterns in `state/global_actions.rs`
     (`command_palette_entries`, `toggle_command_palette`).
4. `ConsoleClient` scoping
   - Each window already owns a `ConsoleClient`; confirm no shared/global
     base-URL state leaks between windows when two windows point at different
     servers (audit `set_base_url` call sites and any process-wide client
     singletons).
5. Persistence
   - Session-only: no schema change to `state.json` beyond (optionally)
     treating the persisted `active_id` as the cold-start default.

## Verification

- Window 1 on local + Window 2 on staging stay connected independently;
  switching in one never disturbs the other.
- `Cmd+Shift+N` inherits the current window's server; cold start uses the
  persisted default.
- Server add/edit/remove in Settings propagates the list to all windows
  without changing their selections.
- Switching still resets that window's tabs/caches and reloads from the new
  backend (existing `activate_environment` semantics preserved).
- Run the relevant test file(s) for touched areas; fix and re-run until
  green before committing.

## Open questions for the user

1. Should per-window servers survive a restart (reopens the out-of-scope
   multi-window persistence decision), or stay session-only? Recommended:
   session-only.
2. New-window default: inherit the current window's server (recommended),
   always the global default, or a small picker at creation time?
3. Switcher UI: command-palette entries (recommended), footer/status
   dropdown, per-window Settings section, or a combination?
4. Editing semantics: add/edit/remove stays global with only selection
   per-window — any objections?
