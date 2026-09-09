# No-project persistence plan (desktop)

## Problem (UI terms)

- You have e.g. `Project A` showing in the bottom picker.
- You click `New Chat` — the new chat still shows `Project A`.
- You open the project picker and choose `No project` — the footer now correctly says `No project`.
- You close that chat tab.
- You re-open the same chat from the sidebar.
- You expect it to still say `No project`.
- But it flips back to `Project A` / the last project.

So `No project` works for the moment, but close + reopen forgets it.

## Repro

1. Select a project (`Project A`).
2. `New Chat`.
3. Picker -> `No project`.
4. Close the tab (`x` or `Cmd+W`).
5. Re-open the same session from the sidebar.
6. Observe: picker shows `Project A` again instead of `No project`.

Fast close/reopen and app restart after step 3 make it more likely.

## Root causes (from code review)

1. **New chat inherits the project** — `create_new_chat` (`apps/desktop/src/state/global_actions.rs:65`) builds `session_project_id = pane_project_id || selected_project_id`. A chat created while `Project A` is active is born with `project_id = A`.
2. **`No project` is memory-only until the server confirms** — `clear_project_for_pane` (`apps/desktop/src/state/projects.rs:216`) moves the tab to the `None` workspace, sets in-memory `selected_project_id = None`, `session.project_id = None`, `cwd = ~/.console/scratch/<id>`, persists `workspace.json`, then fires async `sessions.update(project_id: None, cwd: fallback)` and returns. There is no pending-override flag.
3. **Reopen re-derives from a possibly stale server header** — `load_session_messages_for_pane -> apply_session_header_for_pane -> sync_project_from_session_for_pane` (`apps/desktop/src/state/sessions.rs:315`) resolves `target_id` by `cwd` path match first, then `project_id`. If the step-2 update hasn't landed / failed / early-returned (`pre_move_session == None`), the header still says `A`, so `target_id = Some(A) != pane None` and it overwrites pane + global state back to `A` and refetches branches.
4. **Reopen target resolution prefers stale header** — `target_project_for_session` (`apps/desktop/src/state/sessions.rs:385`) uses stored `project_id`, then `cwd` match. No notion of "user just cleared this session".
5. **Tab open stamps the pane project onto a `None` session** — `open_chat_tab_in_pane` (`apps/desktop/src/state/workspace_panes.rs:448`): `project_id = session.project_id.or(pane_project_id)`. For a known `No project` session (`project_id = None`) this writes the pane's current project (e.g. `A`) onto the tab instead of explicit `None`. It also only syncs pane state when `session_project_id.is_some()`.
6. **Same-workspace reopen doesn't sync/persist** — `select_and_open_session` same-workspace branch (`apps/desktop/src/state/sessions.rs:553`) does `retain_project_tabs + set_tab_project` but never updates `workspace_pane_states[pane].selected_project_id` / `selected_project_id` and never persists, so a drifted pane keeps showing `A`.
7. **Close leaves stale in-memory trees** — `close_workspace_tab` (`apps/desktop/src/state/workspace_panes.rs:701`) closes + persists the live root but does not evict that tab from `project_workspace_roots[*]` caches, so the cached `None` tree can still hold the closed tab while disk has an empty `None` tree.
8. **Server semantics matter** — `SessionService.createSession/updateSession` (`apps/server/api/src/services/session.service.ts:24,83`): explicit `projectId: null` = scratchpad (no project); `undefined` = infer from `cwd` via `getProjectByDir`. Desktop must send explicit `null`, not `undefined`, for `No project`.

## Fix plan

1. **Add per-session pending workspace override.**
   - New field on `ConsoleDesktopApp` (e.g. in `state/app.rs`): `pending_project_override: HashMap<SessionId, Option<ProjectId>>`.
   - Set to `Some(pid)` in `select_project_for_pane`, to `None` in `clear_project_for_pane`.
   - Clear the entry when the `sessions.update` response confirms the header matches, on update error after reconciling, and in `apply_session_header_for_pane` when header `project_id/cwd` already equals the override.
2. **Honor the override in `sync_project_from_session_for_pane` (`state/sessions.rs:315`).**
   - If an override exists for that session, apply it to `workspace_pane_states[pane].selected_project_id` + `selected_project_id` (clear branches, `branch_loaded = true` for `None`), `cx.notify()`, and return early — skip header `cwd/project_id` resolution and the branch fetch.
3. **Honor the override in `target_project_for_session` (`state/sessions.rs:385`).**
   - Check pending override first; for `None` return `None` immediately so reopen computes `target = None` even when the server header is stale. Otherwise keep current `project_id`-then-`cwd` order.
4. **Fix `open_chat_tab_in_pane` (`state/workspace_panes.rs:448`).**
   - When the session is known in `self.sessions`, use its `project_id` verbatim including `None`; only fall back to `pane_project_id` for truly unknown sessions.
   - Mirror that value into `workspace_pane_states[pane].selected_project_id` (both `Some` and `None` cases), not only when `Some`.
5. **Fix `select_and_open_session` same-workspace branch (`state/sessions.rs:553`).**
   - After `retain + set_tab_project`, sync `workspace_pane_states[active_pane].selected_project_id` + `selected_project_id` to `target_project_id` (including `None`), reset branch state accordingly, and call `persist_layout()` + `persist_workspaces()` like the switch branches.
6. **Evict closed tabs from cached workspaces in `close_workspace_tab` (`state/workspace_panes.rs:701`).**
   - After closing in the live root, also `close_matching_tabs` for that `tab_id` across all `project_workspace_roots` values (or refresh the `None` entry from the live root when `selected_project_id.is_none()`), then persist. Keeps memory and `workspace.json` in agreement.
7. **Send explicit `null` for `No project` creation/update.**
   - Verify `create_new_chat` with `No project` active sends `project_id: None, cwd: None` so the server takes the scratchpad path (`dto.projectId === null`) and does not infer via `getProjectByDir`.
   - Verify `clear_project_for_pane`'s `UpdateSessionDto { project_id: None, cwd: fallback }` path updates with zero messages and reconciles the sidebar on the next header refresh.
   - No server schema change expected; desktop-only DTO correctness.

## Verification

- Manual: `New Chat (with A) -> No project -> close -> reopen from sidebar` stays `No project`; fast close/reopen before update completes stays `No project`; restart after clear stays `No project`; normal `A -> B` switch still works; same-workspace reopen keeps footer in step.
- Tests: run only the relevant test file per `AGENTS.md` (e.g. `cd apps/server && bun tests/<name>.test.ts` for session/scratch; desktop `cargo check`). Do not run the full suite unless asked.
- Confirm `workspace.json` `None` entry no longer retains closed tabs and `selected_project_id` round-trips as `None`.

## Out of scope / risks

- No change to message-lock semantics (project/cwd locked once a chat has messages).
- No change to `__default__` workspace id mapping in `persist_layout` / `serialized_workspaces`.
- Risk: holding the override too long would pin a stale `None`; mitigation is clearing it on header match + update error + successful reconcile.
