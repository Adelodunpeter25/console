# Settled chats — a kept-but-out-of-the-way state between done and deleted

Adds a **settled** state for a chat: finished enough that it should stop
taking up space in the active list, but worth keeping. A settled chat is not
deleted, not hidden, and not archived. It moves to a collapsed **Settled**
section at the bottom of the sidebar, still one click away.

The model comes from t3code (`~/Developer/Projects/t3code`): a thread has a
`settledOverride` (`"settled"` or `"active"`) and a `settledAt`, the sidebar
has a `settled` section after pinned/active/working/snoozed, and settling
never deletes anything. The word is deliberate. Console already has
*deleted* (soft-delete, purged after 7 days); this is a different thing and
gets no new vocabulary like "archive".

Target: `apps/server-go/internal/db/schema.go` (column),
`internal/services/session/` (settle, unsettle, list, header),
`internal/routes/sessions.go` (endpoints),
`internal/run/` (unsettle on send), `proto/console/v1/session.proto` +
`proto/testdata/session/` (wire), `apps/desktop/crates/console-core/` (client
service), `crates/console-ui/src/layout/sidebar/` + `src/utils/session_groups.rs`
(Settled section), `crates/console-ui/src/primitives/context_menu/session_context_menu.rs`
(menu), `apps/android/.../feature/home/HomeScreen.kt` (mobile list).

Legend: ✅ done and verified · ◐ done, not verified · ⬜ not done.

| Phase | State | Notes |
| :--- | :--- | :--- |
| 0 — decisions | ✅ | §0 |
| 1 — storage + service | ⬜ | `settled_at` column, settle/unsettle, tests |
| 2 — unsettle on send | ⬜ | one hook in the run start path |
| 3 — API + proto | ⬜ | endpoints, `SessionHeader.settled_at`, fixtures |
| 4 — desktop client | ⬜ | `console-core` service + state |
| 5 — desktop sidebar | ⬜ | Settled section, context menu |
| 6 — Android | ⬜ | Settle action, collapsed Settled group |
| 7 — deferred | — | auto-settle, drag between sections, bulk settle |

---

## 0. Decisions (locked)

These settle the product questions before any code is written. A phase that
disagrees with a decision here is wrong.

1. **Settled is its own state, never `deleted_at`.** The 7-day purge destroys
   any row with `deleted_at` set, so reusing it would eventually delete a chat
   the user meant to keep. Settled gets a separate nullable `settled_at`
   column that no purge, retention sweep, or cleanup ever reads.
2. **Settled chats stay in the normal list.** The server returns them with the
   rest; the client decides where to draw them. There is no "include settled"
   filter and no second list endpoint. Hiding is a presentation choice, which
   keeps the API small and lets quick-open and search find settled chats with
   no extra work.
3. **Sending a message unsettles.** A settled chat that receives a prompt
   returns to active automatically. This is a user decision, and the reason is
   safety: a conversation you are still using must never sit in the collapsed
   section. The rule lives on the server, in the one place a prompt is
   accepted, so every client gets it and no client can forget it.
4. **A running chat cannot be settled.** Settling is for finished work. The
   server refuses while a run is in flight (and while a prompt is queued), with
   a clear error, rather than settling something that is about to produce
   output. Unsettling has no such limit.
5. **Settle and unsettle are idempotent.** Settling a settled chat keeps its
   original `settled_at` (so its position in the section does not jump);
   unsettling an active chat is a no-op. Both succeed for any known id, like
   the existing restore.
6. **One section, always last.** The Settled section sits below everything in
   both sidebar modes (by date and by project). A single placement rule beats
   a per-mode one, and it matches t3code, where the settled section is the
   bottom of the list.
7. **Collapsed by default.** The point is to get it out of the way. The
   collapsed state persists with the other sidebar collapse state.
8. **Sort by `settled_at`, newest first.** Not by last activity: a chat settled
   this morning sits above one settled last month even if the older one was
   more recently edited.
9. **No auto-settle in v1.** t3code can settle old, merged, or closed work on
   its own. That needs rules, settings, and a way to explain itself, and it is
   the part of the feature most likely to surprise someone. v1 settles only on
   an explicit user action.

---

## 1. Storage and service

### Schema

`internal/db/schema.go`, in `InitGlobalDB`, using the same additive migration
loop that already adds `worktree_*` and `thinking_level`:

```go
"settled_at INTEGER",
```

Existing databases gain the column on next start; a new database creates it
with the table. Add it to the `CREATE TABLE IF NOT EXISTS sessions` block too,
so a fresh install and a migrated one end up identical.

No index in v1. The list is capped at 100 rows and the column is only read
while scanning them.

### Types

`types.SessionHeader` gains `SettledAt *int64` (`json:"settled_at,omitempty"`).
Every `SELECT` that builds a header (`ListFiltered`, the single-session read
at `session.go:239`, the one in `updates.go:190`) adds the column and
`scanSessionRows` scans it. These are the same five places `deleted_at` is
already threaded through, and `deleted_at` is the template for the whole
change.

### Service

In `internal/services/session/`, next to `SoftDelete` / `Restore`:

```go
// Settle marks a session settled. Idempotent: an already-settled session keeps
// its original settled_at.
func (s *Service) Settle(sessionID string) (bool, error)

// Unsettle clears the mark. Succeeds for any known id.
func (s *Service) Unsettle(sessionID string) (bool, error)
```

`Settle` uses `COALESCE(settled_at, ?)` so a second call cannot move the
timestamp. It does **not** touch `updated_at`: settling is not activity, and
bumping it would reorder the chat in the date groups it is about to leave.
It refuses with `ErrSessionBusy` when decision 4 applies; the busy check asks
the run service, so the service takes a small `isBusy func(id string) bool`
dependency rather than importing `run`.

`services/session_service.go` (the façade at `:180`) exposes both.

### Tests

`apps/server-go/tests/sessions/` (next to `db_test.go`):

- settle sets `settled_at`; unsettle clears it
- settling twice keeps the first timestamp
- settle does not change `updated_at`
- settling a running session is refused; unsettling one is not
- a settled session still appears in `ListFiltered`
- the purge sweep (`ExpiredDeletedSessions`) never returns a settled,
  non-deleted session
- the migration adds the column to a database created without it

---

## 2. Unsettle on send

The single place a prompt is accepted is `run.Service.StartRun`
(`internal/run/service.go:285`). Queued prompts (`QueuePrompt`) and steering
(`Steer`) are also sends, and a settled chat can receive either, so the hook
goes in a shared helper they all call, not in `StartRun` alone:

```go
// unsettleOnSend clears the settled mark when a prompt arrives. Best-effort:
// a failure here must never block the send itself.
func (s *Service) unsettleOnSend(sessionID string)
```

Best-effort matters: the chat must still answer if clearing the mark fails,
and the failure is logged, not returned. The unsettle happens **before** the
run starts, so the event stream that follows already reflects the chat as
active.

A client that is showing the chat in the Settled section needs to learn it
moved. The run already streams session events; the header it refreshes after
a send carries `settled_at: null`. No new event type in v1.

### Tests

`apps/server-go/tests/` (alongside the run tests):

- `StartRun` on a settled session clears `settled_at`
- `QueuePrompt` and `Steer` do too
- a failing unsettle does not fail the run
- an unsettled session is untouched by the helper (no write, no `updated_at`
  change)

---

## 3. API and proto

### Endpoints

`internal/routes/sessions.go`, beside `/:id/restore`:

| Method | Path | Success | Errors |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/sessions/:id/settle` | `{id, settled: true}` | 404 unknown id; 409 while a run is active |
| `POST` | `/api/sessions/:id/unsettle` | `{id, settled: false}` | 404 unknown id |

Same `{success, data}` envelope as every other route. The 409 carries a message
the UI can show as written ("Finish or stop the run before settling this
chat.").

### Proto

`proto/console/v1/session.proto`:

```proto
message SessionHeader {
  ...
  SessionWorktree worktree = 14;
  optional int64 settled_at = 15;   // unset = active
}

message SessionSettleResponse {
  string id = 1;
  bool settled = 2;
}
```

Field 15 is optional and additive: older clients ignore it and every existing
fixture still decodes. Regenerate with `make proto`.

### Fixtures

`proto/testdata/session/`: add `settle.json`, `unsettle.json`, and a
`header_settled.json`. The Go, Rust, and Kotlin fixture tests all decode the
shared directory, so a field one side gets wrong fails on all three.

### Tests

`tests/api/` (next to `favorites_test.go`): both endpoints against their
fixtures, the 404, the 409, and that `GET /api/sessions` includes
`settled_at` for a settled row and omits it otherwise.

---

## 4. Desktop client

`crates/console-core/src/services/session.rs`: `settle(id)` and
`unsettle(id)`, mirroring `restore`. `SessionHeader` already comes from
`console_proto`, so `settled_at` arrives with the regenerated types.

`src/state/`: two actions, `settle_session` and `unsettle_session`. Each
updates the local header immediately (optimistic, like the favorite toggle)
and rolls back and shows the server's message on failure. A 409 is expected
and must read as a normal message, not an error banner.

The session list the sidebar reads is already `Rc<Vec<SessionHeader>>`, so the
Settled section is derived from it, not a second store.

---

## 5. Desktop sidebar

### Grouping

`crates/console-ui/src/utils/session_groups.rs`:

- Add `partition_settled(len, is_settled)` returning `(active_positions,
  settled_positions)`. The existing `group_indices_by_date` and
  `group_indices_by_project` are then run on the **active** positions only,
  so neither grouping function changes and both modes get settled handling
  from one place.
- Settled positions are sorted by `settled_at` descending (decision 8).

### Section

`layout/sidebar/`: a `settled_header` built like `group_header`, with a count
("Settled · 12") and the same chevron and click-to-toggle. It renders after the
last active group in either mode and is collapsed by default. Its collapse state
joins `collapsed_groups` in `src/state/layout.rs`; because that set is keyed by
`SessionDateGroup` today, give the section its own persisted boolean rather
than stretching the enum.

The section is omitted entirely when there are no settled chats.

Rows inside it are the ordinary session row, slightly dimmed, so a chat does not
look different once opened.

### Context menu

`primitives/context_menu/session_context_menu.rs` already has Rename and
Delete. Add **Settle** above Delete for an active chat and **Unsettle** for a
settled one (never both). Settle is disabled, with a tooltip, while the chat
is running.

### Quick-open and search

Unchanged by design (decision 2): settled chats are in the list, so they are
found. The palette may show a small "Settled" tag; optional, not required.

### Tests

`crates/console-ui/tests/`:

- `partition_settled` splits correctly and preserves each side's order
- grouping the active side leaves settled chats out of every date and project
  bucket
- settled chats sort by `settled_at`, newest first, regardless of `updated_at`
- an empty settled side yields no section

---

## 6. Android

`HomeScreen.kt` lists chats today; add a collapsed **Settled** group at the
bottom using the same list item composable. Long-press (or the existing chat
overflow menu) gains **Settle** / **Unsettle**. The repository calls the two
endpoints and updates the in-memory header optimistically, same shape as
`setFavorite`.

The chat screen needs no change: sending a message unsettles server-side
(decision 3), and the next header refresh moves the chat out of the group.

### Tests

`apps/android/tests/`: a `*FixtureTest` decoding the new `header_settled.json`
and `settle.json`, and a pure test of the partition (settled chats leave the
active list and sort by `settledAt`). Per `AGENTS.md`, only the relevant test is
run, and `assembleDebug` is the build check.

---

## 7. Deferred

- **Auto-settle** after N days, or when a branch's pull request merges (t3code's
  `autoSettleAfterDays` / `autoSettleOnMerge`). Needs settings, a visible
  reason on each auto-settled chat, and an opt-out; revisit once manual
  settling has real use.
- **Drag between sections** (t3code's drop-to-settle). A menu action is enough
  first; dragging needs per-mode drop targets.
- **Bulk settle** ("settle everything older than a week").
- **Pinned section interplay.** Console has no pinned section; if one lands, a
  pinned chat should not be settle-able without unpinning.

## Risks

- **The purge.** The whole design rests on `settled_at` being invisible to
  every cleanup path. The test in §1 that asserts the sweep never returns a
  settled, non-deleted row is the guard, and it should stay.
- **Settling during a run.** The busy check is a race by nature: a run can
  start between the check and the write. The write is therefore conditional
  in the same statement where practical, and the client treats a later
  unsettle-on-send as the correction, which decision 3 already guarantees.
- **Sidebar virtualization.** The list is virtualized and positions are
  resolved on demand. The partition must hand the virtualizer a flat position
  list per section, not clone headers, or a large history gets slow.
- **Mobile parity drift.** Three clients render the same rule. The shared
  fixtures catch wire drift; the placement rule (always last, collapsed,
  sorted by `settled_at`) has to be restated in the Android test, not assumed.
