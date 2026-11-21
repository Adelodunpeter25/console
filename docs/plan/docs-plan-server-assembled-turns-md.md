# Server-Assembled Turns & Turn-Based Pagination

## Problem

`session.Load(id, limit, before)` in `apps/server-go` pages raw LLM rows by rowid (`rowid < before ORDER BY rowid DESC LIMIT n+1`). One tool call is 2 rows, so a turn with 25+ tool calls overflows the 50-row page. Every client then:
1. Rebuilds runs itself (`ReconstructRuns.kt` on Android, transcript grouping on Desktop): pairing tool calls with results and splitting on wake records.
2. Guesses wake boundaries with a 5s timestamp heuristic, because the server strips synthetic wake rows (`visibleMessages`) and sends only `(message, at)` (`extractWakeRecords`). Both helpers only see the current page, so a page without the synthetic row loses its split.
3. Handles pages that start mid-run or show zero bubbles, and re-runs reconstruction over all history when an older page is prepended.

## Goal

`apps/server-go` is the single source of truth for transcript structure:
- History is paged by **whole turns**, never rows.
- The server returns pre-assembled `TranscriptTurn` objects. Clients render them directly.
- The existing run-activity UI (events, thinking/text/tool rows, wake dividers) is kept as is; only where it is computed moves.

## Decisions

- **Whole turns, always.** A turn is never split across pages, even with 200 tool calls. Size is controlled by omitting tool-result bodies (below), not by cutting turns.
- **Turn boundary = a user row** (real or synthetic wake). Synthetic rows already carry `wake_label`, so no timestamp heuristic is needed; boundaries are exact.
- **Cursor = rowid of the first row of the oldest returned turn** (same type as today's `next_cursor`). `has_more` means older turns exist.
- **Boundary lookup:** find the rowid of the (N+1)th-from-last user row before the cursor, then read rows in that range. Add an index on user rows (`messages(role)`, partial on `role='user'` if supported) so this is an index walk, not a table scan. No backfill, no new column.
- **Tool results omitted from turns.** Each tool call carries a small result summary (status, error flag, size). The full result body is fetched on demand when the user expands the call, via a new endpoint. This keeps first-open payloads small, especially on mobile, and avoids materializing every split message.
- **Historical status is `completed`.** No runs table exists (`messages` = id, role, content, created_at), so aborted/error cannot be recovered for past turns. `elapsed_ms` is last row time minus first row time. Only the open turn can be `working`.
- **Open turn is server-owned.** If `runs.IsActive(id)`, the tail turn is returned with status `working` and the activity persisted so far. Clients stop doing `withLiveRun` position patching; SSE events only append to the tail turn.
- **No backward compatibility.** Apps are always up to date: `messages` and `wake_records` are removed from the session page and bootstrap in the same change. Server, Android and Desktop ship together.
- Raw message table is unchanged; the agent loop keeps reading exact model context.

## Design

### 1. Proto (`proto/console/v1/session.proto`, `message.proto`)

- `TranscriptTurn`: `id` (first row id), `created_at`, `optional UserMessage user_message` (real user turns), `optional string wake_label` (wake turns), `optional RunActivity run`, `repeated AssistantMessage responses` (visible final text/artifact messages).
- `RunActivity`: `run_id`, `started_at`, `elapsed_ms`, `RunStatus status` (enum: WORKING, COMPLETED, ABORTED, ERROR), `repeated RunActivityEvent events` (ordered thinking / text / tool_call).
- Tool call events carry a `ToolResultSummary` (not the body); the full `ToolResult` comes from the on-demand endpoint.
- `SessionMessagesPage` and `SessionBootstrapResponse`: replace `messages` and `wake_records` with `repeated TranscriptTurn turns`; keep `has_more` / `next_cursor`.
- Regenerate Go, Rust, Kotlin bindings. Add fixtures under `proto/testdata`.

### 2. Server (`apps/server-go`)

- `internal/services/session/`: `LoadTurns(sessionID, limitTurns, beforeCursor)` using the boundary lookup above, then `AssembleTurns(rows)`, a port of `reconstructRuns` (tool call/result pairing, wake splitting from synthetic rows, hidden tool-transport filtering).
- Edge cases: tool call with no result (crash) stays unresolved; turn with no final response; synthetic wake turn mid-run.
- `GET /api/sessions/:id` and `/bootstrap` return `turns`; the open turn is marked `working` when the run is active.
- New `GET /api/sessions/:id/tool-results/:callId` for the full result body.
- Remove `extractWakeRecords` / `visibleMessages` from routes once unused.

### 3. Android (`apps/android`)
- `ChatSessionState` / `ChatTranscript.kt` render `List<TranscriptTurn>`.
- Delete `ReconstructRuns.kt`, `withLiveRun`, `visibleMessages()`, `buildUserRunMap()`, wake-record merging; fetch the tool result on expand.
- SSE (`attachServerRun` / `startRun`) updates only the tail turn.

### 4. Desktop (`apps/desktop`)
- `console-core` session types and `TranscriptView` consume `turns`.
- Remove client-side tool-transport filtering and wake-record splitting; fetch the tool result on expand.

## Testing
- Go tests in `apps/server-go/tests/sessions/` against shared fixtures: 25+ tool-call turn, wake mid-turn, missing tool result, page boundary exactly on a turn, cursor round trip, open turn `working`.
- Android `*FixtureTest.kt` decoding the new fixtures.
- Rust test in `console-core/tests/`.

## Order of work
1. Proto + regenerate + fixtures.
2. Server assembler, pagination, index, endpoint, tests.
3. Android migration.
4. Desktop migration.
5. Delete dead code.
