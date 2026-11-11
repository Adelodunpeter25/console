# Transcript scroll persistence (desktop)

## Goal
Remember each chat's transcript scroll position reliably across every way of switching chats, and across app restarts.

## Current state
- `transcript_scroll_positions: HashMap<session_id, {row_index, offset_in_row, at_tail}>` lives in memory only (`src/state/types.rs:153`, init `src/state/app.rs:624`).
- Written by `save_transcript_scroll_position` (`src/state/transcript_scroll.rs`), read in `load_session_messages_for_pane` (`src/state/sessions.rs:~1187`, restored at `~1270` via `TranscriptView::restore_scroll_anchor`).
- `state.json` (`src/persistence/store.rs`, `StorageDocument`) has no scroll section; quit only flushes drafts (`app.rs:413`).

## Bugs to fix
1. **Tab-bar / ⌘⇧P / Option+1-9 switch never saves.** `activate_workspace_tab` (`src/state/workspace_panes.rs:400`) calls `set_messages(Vec::new())` and reloads without saving the outgoing chat.
2. **Save targets the active pane, not the pane acted on.** `save_transcript_scroll_position` uses `active_pane_id` + `selected_session_id`. `close_tab_and_sync_pane` (`:34`) and `move_workspace_tab_to_pane/split` (`:720`, `:790`) can snapshot the wrong pane. `selected_session_id` is also `None` while a file/terminal tab is in front.
3. **Keyed by session only.** Same chat in two panes shares one entry.
4. **Row index is window-relative.** After older pages are prepended, a saved index can exceed the freshly loaded 50-message window and clamp to the bottom (`apply_scroll_anchor`, `transcript_view.rs:~725`).
5. **Streaming placeholder row** (`row_count`, `transcript_view.rs:~669`) shifts indices by one when saved mid-run.
6. **Entries never evicted** on session delete (only on environment switch, `environments.rs:349`).
7. **`transcript_for_pane` falls back to the main transcript** for unknown pane ids (`workspace_tabs/chat.rs:296`), so a save can snapshot the wrong view.

## Design decisions
- **Anchor identity: message id + offset**, not row index. Store `anchor_message_id` (user/assistant `id`), `offset_in_row`, `at_tail`. On restore, find the row of that id in the loaded window; if absent (older than the loaded page, or no id), fall back to tail. Keep `row_index` only as an in-memory fallback for messages without ids (optimistic user messages).
- **Keying: per session** (keep current behavior; last writer wins when a chat is open in two panes). Revisit per-pane only if it proves a problem.
- **Persist only from the main window** (same rule as `persist_layout`).

## Steps

### Phase 1 — Fix in-session correctness
1. Change the save API to take explicit `(pane_id, session_id)` everywhere; keep the no-arg wrapper only where active pane is genuinely correct. Resolve session via `active_session_for_pane(pane_id)` rather than `selected_session_id`.
2. In `activate_workspace_tab`, before `set_messages(Vec::new())`, save for `(pane_id, prev_sid)` when `prev_sid != sid`.
3. Update `close_tab_and_sync_pane`, `move_workspace_tab_to_pane`, `move_workspace_tab_to_split`, `close_workspace_pane` to save the pane being acted on (close_workspace_pane currently saves nothing).
4. Make `scroll_anchor()` ignore the streaming placeholder (clamp to `messages.len()-1`).
5. Remove the entry when a session is deleted.
6. Add `TranscriptView` helpers: `anchor_message_id()` (id of the row at top, if any) and `row_for_message_id(id)`; extend `TranscriptScrollPosition` with `message_id: Option<String>` and use it in restore, falling back to clamped `row_index`.

### Phase 2 — Persist across restarts
1. `src/persistence/store.rs`: add `PersistedTranscriptScroll { message_id: Option<String>, offset_in_row: f32, at_tail: bool }` and `PersistedTranscriptScrollState { positions: HashMap<String, PersistedTranscriptScroll> }`; add `transcript_scroll: Option<...>` (optional, so existing files still parse; keep `STORAGE_VERSION = 1`). Add `save_transcript_scroll` / read via `read_document`.
2. `src/state/app.rs` (~line 100, where `store_doc` is read): seed `transcript_scroll_positions` from the document (main window only).
3. Write path: debounced save (same pattern as `persist_workspaces`, `layout.rs:97+`) after each snapshot, plus a flush in the existing `cx.on_app_quit` subscription (`app.rs:413`). On quit also snapshot every pane's visible chat first.
4. Prune: drop entries for sessions that no longer exist after the session list loads (`bootstrap.rs`), and cap the map (e.g. 200 most recent).
5. Environment switch keeps clearing in-memory entries; also clear the persisted ones for that environment (or key persisted entries by environment id).

## Tests
Per AGENTS.md, no inline `#[cfg(test)]`; put tests in `tests/`.
- `crates/console-ui/tests/chat/`: pure helpers (`row_for_message_id`, id-anchor fallback to tail when id is missing, placeholder row excluded from anchor).
- `apps/desktop` store tests: round-trip of `PersistedTranscriptScrollState` through `StorageDocument` JSON, and a file without the field still parses.
- Run only the relevant file, e.g. `cd apps/desktop && cargo test -p console-ui <test_name>`.

## Manual verification
- Two chat tabs in one pane: scroll chat A up, switch via tab bar, switch back → same place.
- Split panes: close a tab in the unfocused pane → focused pane's position unaffected.
- Scroll far up (load older pages), switch away and back → lands on the same message or tail, never a different message.
- Quit and relaunch → each open chat restores to its message; chats at the bottom stay at the bottom.
- Delete a chat → its entry disappears from `state.json`.

## Out of scope
- Persisting scroll for file/diff viewers and terminals.
- Per-pane (rather than per-session) positions.
- Server-side or cross-device scroll sync.
