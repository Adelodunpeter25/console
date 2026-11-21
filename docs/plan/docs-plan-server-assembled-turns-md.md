# Server-Assembled Turns & Turn-Based Pagination

## Problem

Currently, `sessions.Load(id, limit, before)` in `apps/server-go` paginates over raw LLM conversation rows (`UserMessage`, `AssistantMessage` with `ToolCallPart`, `ToolResultMessage`, final `AssistantMessage`). Because a single tool call produces 2 raw rows, a turn with 25+ tool calls exceeds the 50-row initial page.

This forces every client (Desktop and Android) to:
1. Duplicate run-reconstruction logic (`ReconstructRuns.kt` on Android, transcript grouping on Desktop) to pair tool calls with tool results and split on `wakeRecords`.
2. Handle partial-turn page boundaries (pages that start mid-run or contain zero visible bubbles).
3. Re-run reconstruction across the entire loaded history whenever an older page is prepended.

## Goal

Make `apps/server-go` the single source of truth for UI transcript structure by:
1. Paginating session history by **turns** rather than raw LLM rows.
2. Returning pre-assembled **`TranscriptTurn`** objects (prompt/wake divider + `RunActivity` + final assistant response) alongside or in place of raw `messages`.

---

## Proposed Design

### 1. Protobuf Schema (`proto/console/v1/session.proto`, `message.proto`)

Add turn-level types to `console.v1`:

- `TranscriptTurn`:
  - `string id` (stable turn ID, e.g. user message ID or wake record ID)
  - `int64 created_at`
  - `optional UserMessage user_message` (present for user-initiated turns)
  - `optional string wake_label` (present for background-work wake turns)
  - `optional RunActivity run` (pre-assembled activity for this turn)
  - `repeated AssistantMessage responses` (visible text/artifact assistant messages)
- `RunActivity`:
  - `string run_id`
  - `int64 started_at`
  - `int64 elapsed_ms`
  - `string status` (`"working"`, `"completed"`, `"aborted"`, `"error"`)
  - `repeated RunActivityEvent events` (ordered `thinking`, `text`, and `tool_call` with paired `ToolResult`)
- Extend `SessionDetailResponse` and `SessionBootstrapResponse` with:
  - `repeated TranscriptTurn turns`
  - Keep `messages` / `wake_records` temporarily for backwards compatibility during rollout, then deprecate/remove once both clients migrate.

### 2. Server-Side Turn Assembly & Pagination (`apps/server-go`)

- **Storage:** Keep the raw LLM message table unchanged so provider/agent loops continue to read exact model context.
- **Turn Assembler (`internal/services/session/`):**
  - Port the `reconstructRuns` + `wakeRecords` splitting + tool-call/result pairing logic into a single Go function tested against shared `proto/testdata` fixtures.
  - Group raw rows into `[]TranscriptTurn`.
- **Turn-Based Pagination:**
  - `LoadTurns(sessionID, limitTurns, beforeTurnCursor)` slices by **turns** (e.g. `limit = 20` turns), ensuring every page starts on a clean turn boundary and never splits a run in half.
  - `GET /api/sessions/:id` and `GET /api/sessions/:id/bootstrap` populate `turns`, `has_more`, and `next_cursor`.
  - Optional optimization: omit bulky `ToolResult` payloads from initial `turns` and fetch them on demand when a user expands a specific tool call.

### 3. Client Migration

#### Phase 1 — Server + Proto
1. Define `TranscriptTurn` and `RunActivity` in `proto/console/v1/` and regenerate Go, Rust, and Kotlin bindings.
2. Implement `AssembleTurns` and turn-based pagination in `apps/server-go`, with unit tests in `apps/server-go/tests/`.
3. Populate `turns` in `GET /api/sessions/:id` and `GET /api/sessions/:id/bootstrap`.

#### Phase 2 — Android (`apps/android`)
1. Update `ChatSessionState` and `ChatTranscript.kt` to render `List<TranscriptTurn>` directly.
2. Remove `ReconstructRuns.kt`, `visibleMessages()`, `buildUserRunMap()`, and `mergeLiveHistory()` complexity.
3. Live SSE events (`attachServerRun` / `startRun`) only update the tail `TranscriptTurn`'s `run` and `responses`.

#### Phase 3 — Desktop (`apps/desktop`)
1. Update `console-core` session types and `TranscriptView` to consume `turns` directly.
2. Remove client-side hidden-tool-transport row filtering and wake-record splitting.

#### Phase 4 — Cleanup
1. Remove legacy `messages` / `wake_records` fields from `SessionDetailResponse` and `SessionBootstrapResponse` once all clients are updated.
