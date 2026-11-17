# Wake-turn run boundaries on desktop (background-work resume)

## Context

When background work finishes while a session is idle, the server wakes the session with a
**synthetic turn** (`Prompt{Synthetic: true}` → `wakeIfIdle`, `apps/server-go/internal/run/service.go:75-83`).

The synthetic user message is persisted but flagged `synthetic` and stripped from history
responses (`visibleMessages`, `apps/server-go/internal/routes/proto_json.go:46`;
`services/session/prompts.go`). On the wire the turn is announced by a
`backgroundWorkContinue` frame carrying a human label — e.g. *"Bash tool completed, continuing
agent"* (`wakeMessage`, `apps/server-go/internal/run/background.go:101-116`; broadcast at
`turns.go:596-599`) — followed by ordinary model/tool events. A silent `resumed`
notification (`kind: "resumed"`, `notify.go:87-94`) tells clients to re-attach.

**Mobile parity**: the same wake-turn split is implemented on Android — see Workstream 5.

### What desktop does today (the bug)

1. `notifications.rs:51-66` sees `resumed`, waits 400ms, calls `attach_resumed_session` →
   `attach_session_run_for_pane` (`run.rs:1388`).
2. `attach_session_run_for_pane` seeds the elapsed timer from
   `transcript.latest_user_prompt_at()` — the **original prompt's send time** (`run.rs:1398-1408`).
   `running_sessions[session_id]` gets that same timestamp, so the sidebar "Working for Ns" label
   (`execution.rs:19-22`, `sidebar/mod.rs:877`) also counts from the original prompt. A bash job
   that ran 4 minutes shows "Working for 4m+" the instant the wake turn starts.
3. `process_agent_event` handles `BackgroundWorkContinue` by upserting work items only; the
   `message` label is **dropped** (`run.rs:1175-1179`). No divider, no banner.
4. The wake turn's assistant/tool events stream into the transcript with no boundary marker.
   `compute_tool_activity_for_user` (`tool_activity.rs:25-127`) collects everything after the
   last `AgentMessage::User` into **one** run block, so the wake turn's tool calls merge into the
   previous prompt's block: its header flips to the live `WorkingIndicator`
   (`toolcalls.rs:823-828`), auto-expands (`toolcalls.rs:779-783`), and inherits the wrong timer.
5. After settle, `set_messages` reloads history where the synthetic user message is filtered out,
   so the merged look **persists** — the wake turn's tool calls sit inside the earlier turn's
   "Worked for Ns" block forever.

Net effect: an invisible synthetic turn borrows the last user message's identity, inherits its
timer, and its tool calls pile into that message's run block.

## Decisions

1. **Desktop-only change.** No proto or server changes. The wire already carries everything
   needed: `backgroundWorkContinue.message` (label) live, and per-message `createdAt` timestamps
   for reconstruction after reload.
2. **Boundary = a new `AgentMessage::RunBoundary` transcript marker**, not a rendered row of its
   own. It splits run-block grouping; a separate lightweight divider row renders between the
   resulting blocks. The wake turn itself still never renders as a user bubble (server contract
   preserved).
3. **Divider copy = the server's `message` verbatim** ("Bash tool completed, continuing agent",
   "Subagent completed, continuing agent", etc.).
4. **Wake run blocks are collapsed by default once finished.** This falls out of existing
   semantics for free: a non-working `ToolCalls` block is collapsed unless its `run_id` is in
   `expanded_runs` (`toolcalls.rs:779-783`), and `set_messages` clears `tool_calls_state` on every
   reload (`messages.rs:161`). No extra state needed — but see the live-session note in
   Workstream 3.
5. **Timer fix is part of this change**: a wake re-attach seeds `started_at` from the wake
   boundary's timestamp (≈ now), not `latest_user_prompt_at()`. This fixes both the transcript
   "Working for Ns" and the sidebar label, which share `running_sessions`.
6. **Accepted limitation**: a wake turn producing no tool calls renders its text as a normal
   assistant bubble with no divider (same as today).

## Workstream 1 — `AgentMessage::RunBoundary` variant (console-core)

- `apps/desktop/crates/console-core/src/types/agent.rs`
  - Add variant:
    ```rust
    /// Client-side marker splitting run-block grouping where a server-initiated
    /// wake turn begins. Never serialized to or received from the server.
    #[serde(rename = "runBoundary", skip_serializing)]
    RunBoundary {
        /// Divider label (the backgroundWorkContinue message).
        #[serde(skip_serializing)]
        label: String,
        /// Wake time, unix seconds — seeds the segment's "Working for Ns".
        #[serde(skip_serializing)]
        at: Option<i64>,
    },
    ```
    `skip_serializing` keeps it out of any accidental round-trip; `Deserialize` can derive a
    default or the variant can be `#[serde(skip_deserializing)]`-adjacent — simplest is to give
    the fields `#[serde(default)]` so a stray `{"runBoundary":{}}` decodes harmlessly. It is never
    produced by `from_proto`, so no proto path changes.
- Every exhaustive `match` on `AgentMessage` needs a `RunBoundary` arm. Known sites:
  - `console-ui/src/chat/row.rs` (`transcript_row` match ~line 69, `is_hidden_tool_transport`
    ~line 266): render `div()` (invisible spacer row, same as hidden tool transport) — or better,
    return an empty `div().into_any_element()` early like `is_hidden_tool_transport` does so no
    padding is reserved.
  - `console-ui/src/chat/transcript_view/messages.rs`: `messages_match` (~line 292) — two
    boundaries match iff `at` and `label` match (used by prepend-merge dedup; boundaries are
    client-inserted so this path is rare but must not panic).
  - `console-ui/src/chat/transcript_view/mod.rs`: `message_id_at` / `row_for_message_id`
    (~lines 53-70) — `RunBoundary` has no id → `None`/`false`.
  - `console-ui/src/chat/transcript_view/rows.rs`: `row_count` (~line 60) — see Workstream 3.
  - `console-ui/src/chat/transcript_view/tool_activity.rs` — see Workstream 2.
  - `console-ui/src/chat/turn_changes.rs` (~lines 51-96): boundary is not `User`/`Assistant`;
    verify `take_while`/`all` logic still terminates run scanning correctly (a boundary inside a
    turn's message span should act like a non-assistant message — check whether it needs to break
    the scan; likely treat as transparent, matching `ToolResult`).
  - `console-ui/src/chat/transcript_view/prompts.rs`, `streaming.rs`, `app.rs::user_prompt_history`:
    all filter on `User`/`Assistant`/`ToolResult` with wildcard or specific arms — audit each;
    `RunBoundary` must be ignored (it is not a prompt, not assistant content, not a tool result).
  - `streaming.rs` `last_mut()` matches: a `RunBoundary` can never be `messages.last_mut()` when
    assistant parts stream (boundary is inserted before the wake turn's first part), but the
    `else` branches push a new `Assistant` message — verify no path pushes onto a boundary.

## Workstream 2 — Segment-aware run grouping (console-ui transcript)

`compute_tool_activity_for_user` (`tool_activity.rs:25-127`) currently returns
`Option<(Vec<ActivityEvent>, Option<i64>, u64, bool)>`. Change it to return a **Vec of blocks**:

```rust
pub(crate) struct RunActivityBlock {
    pub events: Vec<ActivityEvent>,
    pub started_at: Option<i64>,
    pub elapsed: u64,
    pub working: bool,
    /// Divider label shown before this block; None for the first segment.
    pub divider_label: Option<String>,
    /// Stable block id for ToolCallsState, e.g. "run-{user_index}-seg-{n}".
    pub run_id: String,
}
```

- Iterate `messages[user_index+1..]`; a `RunBoundary` closes the current segment and opens the
  next (record `label`/`at`). `User` still terminates the whole scan.
- Per segment: same event collection as today (assistant parts with tool calls → timeline events,
  `ToolResult` → result map). Segments with no tool calls produce no block (accepted limitation).
- `working` is true **only** for the last block, only when `is_streaming` and the transcript's
  last message is not a `RunBoundary` (a boundary as the literal last message means the wake turn
  hasn't produced output yet — nothing should show "Working" inside a block; the trailing
  `WorkingIndicator` row covers that, see Workstream 3).
- `started_at`: first segment → `user_started_at` (existing behavior); later segments → the
  boundary's normalized `at`, falling back to `user_started_at`.
- `ended_at`/`elapsed`: per segment, from that segment's message `created_at`s (existing
  `normalize_unix_timestamp` path).
- `run_id`: `run-{user_index}` for segment 0 (unchanged, preserves existing
  `expanded_runs`/`collapsed_runs` keys); `run-{user_index}-wake-{n}` for wake segments.
- Cache: `tool_activity_cache` is keyed by `user_index` and already versioned
  (`tool_activity_cache_version`); boundary insertion must bump/invalidate — see
  `invalidate_activity_cache` vs `invalidate_latest_activity_cache` (`rows.rs:36-55`). Live
  boundary insertion happens once per wake; simplest correct move is a full
  `invalidate_activity_cache()` in the insert path.

`row.rs` `transcript_row` (~lines 126-183): replace the single `activity` render with a loop
over blocks — for each block after the first, render a divider row (label text, styled like the
existing `Worked for Ns` summary: `text_size(11.5)`, `text_tertiary`/`text_ghost`, small vertical
padding, maybe a leading `↻` or `IconName::RefreshCw`-style glyph if one exists — check
`app_icon`/`IconName` for a suitable icon) then the `ToolCalls` element. `ToolCalls::new`
already takes `(run_id, entries, working, started_at, elapsed, state)` — pass the block's values
unchanged.

## Workstream 3 — Boundary insertion, live + reload, and the timer fix

### Live path

- `ConsoleDesktopApp` gains `wake_records: HashMap<String, Vec<WakeRecord>>` where
  `WakeRecord { message: String, at: i64 /* unix secs, normalized */, raw_created_at: i64 }`.
  Declare in `src/state/types.rs` (~line 222, next to `running_sessions`). Cap ~8 records per
  session (FIFO). Clear the session's records in the `session_deleted` handler
  (`notifications.rs:94-124`) and wherever `running_sessions.clear()` happens
  (`environments.rs:364`).
- `process_agent_event` `BackgroundWorkContinue` arm (`run.rs:1175-1179`):
  1. Always append a `WakeRecord` for `run_session_id` (needed for reload reconstruction even if
     the pane isn't showing this chat).
  2. If `pane_shows_run`: insert `AgentMessage::RunBoundary { label: message, at: Some(now) }`
     at the **end** of `transcript.messages` (wake output hasn't streamed yet — `BackgroundWorkContinue`
     is broadcast before `TurnStart`, `turns.go:596-602`), bump `content_revisions`, and
     `invalidate_activity_cache()`. Add a `TranscriptView::push_run_boundary(label, at, cx)`
     helper in `streaming.rs` or `messages.rs` that does exactly this.
- `attach_session_run_for_pane` (`run.rs:1388`): add a `wake_at: Option<i64>` parameter.
  - `now = wake_at.or(latest_user_prompt_at).unwrap_or(now)` — i.e. prefer the override, else the
    existing prompt-time fallback, else now.
  - `notifications.rs` `attach_resumed_session`: pass
    `self.wake_records.get(session_id).and_then(|v| v.last()).map(|r| r.at)` — falls back to
    `None` → prompt time if the record is somehow missing (still wrong-but-no-worse).
  - Other callers (`sessions.rs:1792` bootstrap-attach, `floating_composer.rs:1192`
    worktree-launch attach) pass `None` — unchanged behavior. Rationale: those paths attach to a
    run that may have started long ago; "now" would understate, prompt-time is the existing
    approximation. Only the `resumed` notification path *knows* a wake just began.
- This single `started_at` feeds `set_session_running` → `running_sessions` → sidebar label AND
  `resume_streaming` → transcript `WorkingIndicator`/block timer. Both fixed at once.

### Reload path (boundary reconstruction)

`backgroundWorkContinue` is a live-only frame; after `set_messages` reloads history the boundary
must be rebuilt from persisted data:

- In `TranscriptView::set_messages` (`messages.rs:123`), after `self.messages = messages`, call a
  new `rebuild_run_boundaries()` that reads `self.wake_records` (see below) and inserts
  `RunBoundary` markers.
- `TranscriptView` stores `wake_records: Vec<WakeRecord>` (same struct, defined in
  console-core or duplicated — prefer defining `WakeRecord` in `console-core::types` next to
  `AgentMessage` so both crates share it). `ConsoleDesktopApp` sets it via a new
  `t.set_wake_records(records.clone(), cx)` call **before** `set_messages` at every load site:
  - `sessions.rs:1726` (`load_session_messages_for_pane`)
  - `run.rs:880` and `run.rs:1521` (post-settle `set_messages` — the settle reload is exactly
    where the live boundary must survive; records are already in app state)
  - Any other `set_messages` callers — grep `\.set_messages\(` and cover all.
- Matching rule: for each `WakeRecord` (sorted by `at`), find the **first** message index `> `
  last real `User` boundary… concretely: first `Assistant`/`ToolResult` message whose
  `created_at` (normalized via `normalize_unix_timestamp`) is `>= record.at` **and** that comes
  after the previous user message. Insert `RunBoundary` at that index. Skip records that match
  nothing (timestamps missing, or the wake turn produced no persisted messages — e.g. it errored
  before persisting; acceptable: no boundary, same as today).
- `prepend_older_messages` (`messages.rs:196`): boundary `message_index` values shift by
  `older.len()` — adjust stored indices (they're all tail-side, prepend is head-side; a simple
  `+= older.len()` on each stored index works since boundaries only ever sit after the oldest
  loaded message… verify: boundaries are always after the *last* user message, and prepended
  messages are all *before* it, so every boundary index shifts by exactly `older.len()`).
- Edge: multiple wake turns → multiple records → multiple boundaries, each matching the first
  message after its `at`. Sort records by `at` and consume in order so an earlier wake can't
  steal a later wake's first message (first-match-wins per record, scanning forward only).

### Trailing working indicator (`rows.rs:60-80`)

`row_count` adds a trailing "Working…" row when `is_streaming && !has_active_tool_activity`.
With boundaries, `has_active_tool_activity` must treat a trailing `RunBoundary` correctly:
- If the last message is a `RunBoundary`, the wake turn hasn't produced tool calls yet →
  `has_active_tool_activity` should be **false** → trailing indicator shows. (Current scan already
  returns false since a boundary isn't an assistant-with-tool-call, but the `skip(user_index+1)`
  range now includes boundary markers — verify the `any()` still sees wake-turn assistant parts
  that arrive *after* the boundary. It does: they're later indices in the same skip range.)
- No change needed beyond the `RunBoundary` match arms; call this out in review.

## Workstream 4 — Tests

Follow AGENTS.md: no inline `#[cfg(test)]`; dedicated `tests/` files.

- `apps/desktop/crates/console-ui/tests/chat/` — new fixture test (mirror
  `transcript_anchor_test.rs` style): build a `TranscriptView` with user → assistant(tool calls)
  → `RunBoundary` → assistant(tool calls) → `ToolResult`; assert `tool_activity_for_user`
  (or its public equivalent) yields **two** blocks with correct `run_id`s, `started_at`s, and
  `divider_label` on the second; assert `working` only on the last block while streaming.
- Boundary reconstruction: feed `set_messages` a history + `wake_records` and assert boundary
  indices land on the first post-wake message; assert a record with no matching timestamp inserts
  nothing.
- `prepend_older_messages` index shift: boundary indices increase by `older.len()`.
- Desktop state: `attach_session_run_for_pane` `wake_at` override → `running_sessions` gets the
  override, not `latest_user_prompt_at` (state-level test if a harness exists; otherwise cover via
  the transcript-level `resume_streaming` started_at assertion).
- Server: **no changes**, no tests needed.

## Workstream 5 — Android parity

Android has the same bug through a different mechanism: runs are `RunActivityState` objects
paired to user messages **positionally** (`ChatScreen.kt:206-216` maps the n-th user message to
the n-th run), and `reconstructRuns` (`ReconstructRuns.kt:12-53`) only opens a new run on a
`UserMessage`. The synthetic user message is stripped server-side, so wake-turn tool calls fold
into the previous run — and `withLiveRun` (`ReconstructRuns.kt:67-110`) reopens that merged run
with `startedAt = msg.createdAt` of the original prompt, so "Working for Ns" has the same
wrong-timer bug. `backgroundWorkContinue` is currently lumped with the other background-work
events and only upserts `session.backgroundWork` (`ChatEvents.kt:324-332`) — the `message` label
is dropped here too.

Note: unlike desktop's `since=None`, Android's `attachServerRun` calls `attach(0, ...)`
(`ChatRepository.kt:778`) which **replays the whole run buffer** — so a wake turn started before
attach still delivers `backgroundWorkContinue`. The live path is more reliable here; the
reconstruction path matters for cold opens.

### Model changes (`core/chat/ChatModels.kt`)

- `RunActivityState` gains `dividerLabel: String? = null` — set on the wake segment's run; the
  UI renders the label row above that run's header (same copy as desktop: server `message`
  verbatim).
- `ChatSessionState` gains `wakeRecords: List<WakeRecord>` mirroring desktop's per-session
  records (`data class WakeRecord(val message: String, val at: Long)`; `at` = event receipt
  time in ms, same "frame arrives before wake messages persist" reasoning as desktop).

### Live path (`core/chat/ChatEvents.kt`)

- Split `backgroundWorkContinue` out of the shared arm at `ChatEvents.kt:324`: on that event,
  append a `WakeRecord` to `session.wakeRecords` (cap ~8, FIFO), then **close the current run**
  (same finalize logic as `sessionEnd`/`turnEnd`: status `Completed`, `finalizePendingToolCalls`,
  `elapsedMs = now - startedAt`) and append a new `RunActivityState(runId = newMessageId(),
  startedAt = now, status = Working, dividerLabel = event.message)`.
  - Guard: only split when the last run is `Working` (a `backgroundWorkContinue` can also arrive
    for work delivered mid-run via `backgroundContext`, where no wake turn happens — check
    `HasUndelivered`/delivery semantics; if unsure, splitting on a non-idle session would be
    wrong, so confirm the event only fires for the synthetic wake path before splitting.
    `turns.go:596` broadcasts it only inside `if dto.Synthetic`, and synthetic turns only start
    via `wakeIfIdle`/`nextTurn` — both require an idle session. The mid-run `backgroundContext`
    claim does NOT emit this event. Safe to split unconditionally on the event.)
- `turnStart` handling (`ChatEvents.kt:175-208`) needs no change: synthetic `turnStart` carries
  no `prompt` text (`TurnStart` broadcast without `Text` at `turns.go:598`), so `prompt` is
  null/blank → falls to the `hasWorking` branch → events keep landing in the wake run. Verify
  the deserializer maps the textless frame to `prompt = null`.

### Attach path (`data/repo/ChatRepository.kt`)

- `attachServerRun` → `withLiveRun` already reopens the last `Working` run and preserves its
  `startedAt`; with the split above, that is the wake run with the correct (receipt-time)
  `startedAt`. No change needed beyond what the event split provides.

### Reconstruction (`core/chat/ReconstructRuns.kt`)

- `reconstructRuns(messages)` gains a `wakeRecords: List<WakeRecord> = emptyList()` parameter
  (default keeps existing call sites compiling; update `ChatRepository` call sites at
  `ChatRepository.kt:412` and `ReconstructRuns.kt:157` to pass `session.wakeRecords`).
- Same matching rule as desktop: for each record (sorted by `at`), find the first
  `AssistantMessage`/`ToolResultMessage` with `createdAt >= record.at` after the current run's
  start; close the current run there and open a new `RunActivityState` with
  `dividerLabel = record.message`, `startedAt = first message's createdAt`.
- `withLiveRun`/`prependOlderMessages`/`mergeLiveHistory` need the same record plumbing —
  `mergeLiveHistory` merges server+local messages; wake records live in `ChatSessionState` so
  they survive; verify `reconstructRuns(merged)` call passes them.

### Pairing/rendering (`feature/chat/`)

This is the part that differs structurally from desktop. `userRunMap`
(`ChatScreen.kt:206-216`) maps the n-th **visible** user message to the n-th run, and
`ChatTranscript.kt:52-56` renders at most one `RunActivity` per user message. A wake run has no
user message, so positional pairing breaks: runs.size > userCount and the extra run would never
render (or shift onto the wrong message).

- Change pairing to **per-user-message run lists**: `userRunMap: Map<Int, List<Int>>` — walk
  `displayMessages` user messages in order; each user message owns the runs that follow it
  until the next user message. Since runs are already ordered and each `UserMessage` boundary
  in `reconstructRuns`/live events closes the previous run, group runs by "which user message
  precedes them": run[0] → user[0], run[1] → user[0] if it's a wake segment…
  Concretely: `RunActivityState` needs to know its owning user message. Simplest: add
  `ownerUserIndex`/`dividerLabel` semantics — a run with `dividerLabel != null` belongs to the
  same user message as the run before it. Build the map by iterating runs: first run after a
  user message starts that user's group; subsequent `dividerLabel` runs append to the same
  group; a run without `dividerLabel` starts the next user's group. (Alternative: store an
  explicit `userMessageKey` on `RunActivityState` at creation — the live path knows the last
  user message; reconstruction knows the index. Pick whichever is less invasive during
  implementation; the dividerLabel heuristic is probably enough since wake runs are the only
  multi-run-per-user case.)
- `ChatTranscript`: for each user message render its run **list** in order — `RunActivity` for
  each, with the divider label row rendered above runs where `dividerLabel != null`
  (small `Text`, `NewTheme.TextSecondary`, matching the desktop divider styling; Tabler has
  `Refresh`/`ArrowBackUp` icons if a glyph is wanted).
- `RunActivity.kt`: `expanded` initial state is `remember(runId, running) { mutableStateOf(running) }`
  — finished runs already default collapsed; wake runs get the same "collapsed once finished"
  behavior for free, matching desktop Decision 4.
- `RunVisuals(runs[runIdx])` call site: render visuals for **each** run in the group, not just
  the first.
- `latestUserIndex == -1 && runs.isNotEmpty() && running` fallback (`ChatTranscript.kt:68-72`)
  stays as-is (no user message at all — can't happen for wake since a synthetic turn requires
  prior history, but keep the guard).

### Tests (`apps/android/tests/`, JVM fixture style)

- `ReconstructRunsFixtureTest`-style test: messages = user → assistant(toolCall) →
  toolResult → assistant(toolCall) → toolResult, plus `wakeRecords` with `at` between the two
  assistant messages → assert two runs, second has `dividerLabel` and correct `startedAt`.
- `ChatEvents` reducer test if a harness exists for event→state (check existing test layout
  first): `backgroundWorkContinue` on a Working session → previous run Completed + new Working
  run with `dividerLabel`.
- Run: `cd apps/android && ./gradlew :app:testDebugUnitTest --tests "<new test class>"` and
  `:app:assembleDebug`.

## Verification

- `cd apps/desktop && cargo test -p console-ui` (transcript tests) and `cargo test -p console-core`
  (message enum compile + any serde tests).
- `cd apps/desktop && cargo build` for the gpui app (compile check across all match sites —
  exhaustive matches will fail loudly, which is the point).
- `cd apps/android && ./gradlew :app:testDebugUnitTest --tests "<new test class>"` and
  `./gradlew :app:assembleDebug`.
- Manual: start a session, run `bash` with `background: true` (or a long command), let the main
  turn settle, wait for completion → observe: divider row with the server label appears, wake
  tool calls group under a **second** collapsed "Worked for Ns" block, "Working for Ns" counts
  from wake time (~seconds), sidebar label matches. Then reload/reopen the chat → boundary and
  split persist. Repeat on Android: same divider + second collapsed run, correct timer.

## Risks / notes

- **Race**: `BackgroundWorkContinue` is broadcast on the run hub before `TurnStart`
  (`turns.go:596-602`), and `attach_run` is called with `since=None` (`run.rs:1425`) so there is
  **no replay** — if the client attaches after the frame was broadcast it never sees it. The
  400ms `resumed` delay makes this unlikely but not impossible. Mitigation: the reload
  reconstruction path covers post-settle; a missed *live* frame degrades to today's merged
  display for that one wake turn. Acceptable; a server-side fix (persisting a wake marker on the
  assistant message) is a possible follow-up, out of scope.
- **Timestamp heuristic**: reconstruction relies on `createdAt >= wake.at`. Clock skew between
  server write time and client record time is nil (both server timestamps; `at` is normalized
  server `createdAt`-adjacent — actually `at` is client receipt time for live records; for
  reconstruction use the record's stored `at` which was `Utc::now()` at event receipt — this is
  **after** the wake turn's first message `createdAt`? No: the frame arrives before any wake
  message is persisted, so receipt time ≤ first wake message's `createdAt`. Safe. For extra
  safety the matcher could use `>= at - 5s` slack; decide in implementation, note here.)
- **`RunBoundary` in `messages`**: it is a fake message — every index-based consumer
  (`message_id_at`, scroll anchors, `jump_to_prompt`, pagination cursors) must treat it as
  id-less and non-prompt. The audit list in Workstream 1 is the mitigation; exhaustive-match
  compile errors will catch most sites.
- **`is_hidden_tool_transport` / row padding**: boundary rows must render zero-height like hidden
  transport rows, or every wake adds a visible gap.
- **Multiple rapid wakes**: records are FIFO-capped; reconstruction consumes in `at` order.
- **Android run pairing**: positional `userRunMap` breaks when a user message owns two runs —
  Workstream 5's per-user run-list grouping is the fix; the `dividerLabel`-heuristic for group
  ownership is the main design risk there (a non-wake future multi-run case would need the
  explicit owner field instead).
