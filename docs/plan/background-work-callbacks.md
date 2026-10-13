# Background Work — callbacks for shells and subagents

## Goal

Give Console the same fire-and-forget orchestration primitive described in OpenCode 2.0: the primary agent can start a long-running shell command or delegated agent, continue its own work, and receive a durable completion notification when the background operation finishes.

The implementation must work across the Go service, the protobuf event stream, and desktop/Android clients without making polling or reconnecting clients unreliable. Existing synchronous subagents and `bashJob` polling remain supported while the asynchronous path is introduced.

## Scope

This plan covers:

- a common, session-scoped background-work lifecycle;
- automatic completion callbacks for background shells;
- non-blocking background subagents;
- event persistence and replay for reconnecting clients;
- cancellation, ownership, authorization, retention, and shutdown;
- desktop and Android presentation;
- tests, migration, observability, and staged rollout.

It does not include a JavaScript code mode, a new service-discovery protocol, or arbitrary remote job execution. Work remains local to the Console server and inherits the requesting session's security boundary.

## Current baseline

The repository already provides most of the required primitives:

- `apps/server-go/internal/services/bash_job_manager.go` starts process-group-isolated jobs, captures bounded output, supports status/output/wait/kill, ownership, timeout, retention, and shutdown cleanup.
- `apps/server-go/internal/agent/tools/bash_tools.go` exposes `background=true`, returning a job ID immediately.
- `apps/server-go/internal/agent/tools/bash_job_tool.go` provides polling and control for those jobs.
- `apps/server-go/internal/agent/loop/subagent.go` runs an isolated nested agent and emits `subagentStart`, `subagentActivity`, and `subagentEnd` events, but the tool call blocks its parent until completion.
- `apps/server-go/internal/run/turns.go` binds both tools to a session event hub and propagates the parent model, provider, tools, setup, approval mode, and path guard to subagents.
- `proto/console/v1/session.proto` and the generated clients already contain subagent lifecycle and live-row types.
- Desktop and Android already render subagent activity and consume session event frames.

The important gap is that background shell completion is not automatically sent through the session event stream, and subagent execution is not detached from the parent turn.

## Design principles

1. **One lifecycle, two backends.** Shells and subagents expose the same start/progress/finish/cancel concepts, while retaining backend-specific details.
2. **Events are authoritative; polling is a compatibility path.** A client may use events for immediacy and existing APIs for full output, recovery, or inspection.
3. **Completion is durable enough to survive reconnects.** A client that disconnects must be able to replay or fetch missed terminal events.
4. **No implicit permission escalation.** Background work inherits the parent session's approval mode, path guard, provider/model constraints, and ownership. It cannot outlive the authorization policy that created it.
5. **Bounded data.** Events contain concise summaries and metadata, not unbounded shell output or full transcripts. Full data stays behind existing output/session endpoints.
6. **Explicit user-visible state.** Detached work is visible, cancellable, and attributable to the originating session.
7. **Preserve current behavior.** Existing synchronous `subagent` calls and `bashJob` actions remain valid during migration.

## Proposed lifecycle model

Add a common internal record, named `BackgroundWork` or equivalent, owned by the session/run service:

```text
work_id        stable ID (`work_...`)
session_id     owning Console session
parent_run_id  run that created it, if available
parent_call_id originating tool call, if available
kind           shell | subagent
status         starting | running | completed | failed | cancelled | timed_out
created_at
started_at
finished_at
summary        bounded human/model-readable result
error          bounded failure description
metadata       backend-specific JSON/protobuf fields
```

The manager owns registration, authorization, lifecycle transitions, cancellation, retention, and event publication. The bash manager and subagent runner remain responsible for process and agent execution respectively.

Every transition publishes an event:

```text
backgroundWorkStarted
backgroundWorkProgress       optional, rate-limited
backgroundWorkCompleted
backgroundWorkFailed
backgroundWorkCancelled
```

Backend-specific lifecycle events continue to be emitted where useful. For example, a subagent still emits its detailed activity events, while the common work event makes it possible for clients to render a unified background-work list.

## API and protocol changes

### Protobuf

Update `proto/console/v1/session.proto` or the appropriate event schema with:

- `BackgroundWorkInfo` for list/detail state;
- `BackgroundWorkStartedEvent`;
- `BackgroundWorkProgressEvent` only if a backend has meaningful bounded progress;
- `BackgroundWorkCompletedEvent` containing status, summary, and backend metadata;
- `BackgroundWorkCancelledEvent`;
- a stable `work_id`, `session_id`, `kind`, timestamps, and parent call/run IDs.

Use additive field numbers only. Regenerate Go and Rust/Kotlin bindings using the repository's existing generation workflow; never hand-edit generated files.

The event envelope must preserve unknown fields and tolerate older clients. Older clients may ignore the new frames and continue using `bashJob` or existing subagent endpoints.

### Server endpoints

Add session-scoped inspection and control endpoints if the existing API conventions support them:

- `GET /api/sessions/:id/background-work`
- `GET /api/sessions/:id/background-work/:workId`
- `POST /api/sessions/:id/background-work/:workId/cancel`

The list/detail response is recovery support, not a replacement for the stream. Enforce session ownership and reject access to another session's work.

### Tool contracts

#### Background shell

Keep `bash(background=true)` and `bashJob`. On start, register a common work record and return both the existing `jobId` and a `workId` (or make them the same only if doing so does not break existing clients). On terminal state, publish a completion event with:

- command and working directory;
- exit code or timeout/cancel status;
- bounded stdout/stderr preview;
- a cursor for retrieving complete output with `bashJob`.

The completion message should tell the model to use `bashJob` for full output rather than embedding large logs.

#### Background subagent

Extend the subagent input with an explicit `background` option, defaulting to `false` for backward compatibility:

```json
{
  "prompt": "Review the retry logic",
  "name": "retry-review",
  "role": "Code reviewer",
  "background": true
}
```

For `background=false`, preserve the current blocking result and detailed lifecycle events. For `background=true`:

1. allocate and register the work/subagent ID;
2. emit start events;
3. launch the nested run under a child context;
4. return immediately with the IDs and recovery instructions;
5. publish activity and terminal events from the worker;
6. retain the final summary for the parent session and clients.

The worker must not recursively spawn subagents unless a later, explicit depth policy permits it. It inherits the parent tool registry snapshot, model/provider, setup, approval mode, approver, and path guard exactly as the current synchronous path does.

## Parent-agent callback semantics

The first implementation should use an event callback, not silently mutate the active model transcript while a provider request is in flight. This avoids provider-specific concurrency bugs and preserves turn ordering.

When a background operation completes:

- publish the event to connected clients;
- persist the terminal work record/event;
- make the result available to the next parent turn through session context;
- optionally enqueue a compact system-side notification for the next turn, marked as asynchronous tool completion rather than user text.

Do not interrupt an active provider stream automatically in v1. A later enhancement may support an interruptible continuation, but it needs explicit provider and agent-loop design.

The notification should be compact:

```text
Background work completed: work_abc (shell)
Status: exited, code 0
Summary: 148 tests passed.
Use the work detail/output API for complete results.
```

Deduplicate by `work_id` so retries, reconnects, and repeated polling cannot cause duplicate model actions.

## Execution and cancellation

- Use a child context for every detached operation.
- Store a cancel function in the manager, never in the wire object.
- `session abort` cancels all work owned by that session, matching current `KillOwnedBy` behavior for shells.
- Server shutdown cancels and waits for workers within a bounded grace period.
- Shell cancellation continues to terminate the entire process group.
- Subagent cancellation must stop the nested provider request and all child tool execution.
- Terminal transitions are idempotent; races between exit, timeout, cancellation, and shutdown must produce one final state and one terminal notification.
- Keep bounded output and result sizes, retention limits, and per-session concurrency limits.

## Authorization and safety

- A background operation is owned by the originating session and cannot be read or cancelled by another session.
- It inherits the parent's path guard and approval ceiling. Background execution must not bypass approval prompts already required by the synchronous executor.
- If an approval is required, the detached worker must either pause awaiting the normal decision channel or fail clearly; it must never auto-approve because it is in the background.
- Record the originating tool call and parent run for auditability.
- Reject new work after the parent session is closed or deleted.
- Validate all client-supplied IDs and never trust a client-provided owner field.
- Do not include secrets or unlimited command output in lifecycle events or logs.

## Server implementation phases

### Phase 1 — event and manager foundation

- Add the internal background-work record/state machine and manager.
- Define transition validation and idempotent terminal handling.
- Add session ownership, cancellation, retention, and bounded summaries.
- Add a publication interface that can target the existing session hub.
- Add unit tests for transitions, duplicate terminal signals, authorization, retention, and cancellation races.

**Exit criteria:** an in-memory fake worker can publish a complete lifecycle and a reconnecting caller can retrieve its final state.

### Phase 2 — background shell callbacks

- Register every `bash(background=true)` job with the common manager.
- Connect `BashJobManager` terminal notifications to the manager/event hub.
- Preserve existing `bashJob` status/output/wait/kill semantics and ownership checks.
- Emit concise completion events with output cursors and previews.
- Add endpoint/list recovery if not already supplied by the manager.

**Exit criteria:** starting a background shell returns immediately; completion appears without polling; full output remains available; timeout, kill, failure, session abort, and server shutdown all publish exactly one terminal event.

### Phase 3 — protocol generation and compatibility

- Add additive protobuf messages and event kinds.
- Regenerate Go, Rust, and Kotlin bindings.
- Add golden fixture files for started, progress, completed, failed, cancelled, and shell metadata.
- Verify old clients can consume streams containing unknown new event types.

**Exit criteria:** protocol tests pass in Go, Rust, and Android fixture tests; generated changes are isolated and reproducible.

### Phase 4 — detached subagents

- Add `background` to subagent input with a false default.
- Refactor `SubagentContext.run` so worker creation and event publication can run under a manager-owned child context.
- Return a non-blocking start result for background calls.
- Preserve synchronous behavior, activity events, usage accounting, approval handling, and recursion protection.
- Store the final summary and expose detail/recovery through the common manager.

**Exit criteria:** the parent turn continues after a background subagent starts; the subagent emits activity and one terminal event; cancellation stops it; final usage and summary are recorded.

### Phase 5 — parent-turn notification and recovery

- Add a session notification queue or equivalent next-turn context provider.
- Deduplicate delivered work IDs.
- Inject only compact completion summaries into a subsequent parent turn.
- Add a detail tool/API for retrieving complete subagent summaries and shell output.
- Define behavior when work finishes while no parent turn is active.

**Exit criteria:** a new parent prompt sees one completion notification and can request details; reconnecting clients see the same terminal state without duplicate notifications.

### Phase 6 — client UX

Desktop:

- Add a unified background-work store keyed by `work_id`.
- Render running/completed/failed/cancelled items in the session inspector.
- Show shell output and subagent summary on demand.
- Add cancel controls with confirmation only where required.
- Keep existing subagent panel and bash-job presentation functional.

Android:

- Decode and reduce new event frames into session state.
- Add a compact background-work banner/list in chat and a detail screen or sheet.
- Show completion/failure notifications without interrupting composition.
- Add cancel/retry/detail actions respecting session ownership.
- Load recovery state when opening a session so work completed while offline is visible.

**Exit criteria:** both clients render live and recovered work consistently, and an older client remains usable.

### Phase 7 — rollout and hardening

- Add metrics for starts, duration, terminal status, cancellations, missed/replayed events, output truncation, and worker counts.
- Add structured logs with session/work IDs but redact command secrets and output.
- Gate detached subagents behind a feature flag initially; background shell callbacks can roll out independently.
- Load-test concurrent jobs/subagents, reconnect storms, and event fan-out.
- Document limits, retention, cancellation behavior, and polling fallback.
- Remove temporary compatibility paths only after clients have adopted the event flow.

## Verification matrix

### Go server tests

Run only targeted tests while implementing:

- `go test ./tests/tools/ -run TestBackgroundShell -v`
- `go test ./tests/tools/ -run TestBackgroundSubagent -v`
- `go test ./tests/tools/ -run TestBackgroundWorkLifecycle -v`
- `go test ./tests/session/ -run TestBackgroundWorkAuthorization -v`
- `go test ./tests/events/ -run TestBackgroundWorkEvents -v`

Test cases:

- start returns before completion;
- completion, failure, timeout, kill, and cancellation each produce one terminal event;
- output is bounded and cursor recovery works;
- session ownership is enforced;
- session abort and server shutdown clean up workers;
- duplicate exit/timeout/cancel races are idempotent;
- subagent inherits model, tools, approval mode, and path guard;
- synchronous subagent behavior is unchanged;
- background subagent cannot recurse or exceed concurrency/depth limits;
- usage and summaries are retained and recoverable.

### Protocol/client tests

- Go/Rust/Kotlin decode golden lifecycle fixtures.
- Unknown event types do not break older decoders.
- Desktop event reducer handles out-of-order progress and repeated terminal frames.
- Android event reducer restores the same final state after reconnect.
- UI tests cover running, success, failure, cancellation, and offline recovery.

### Manual acceptance

1. Start a long-running test with `bash(background=true)`.
2. Continue chatting without calling `bashJob`.
3. Observe a completion event in the session UI.
4. Open full output using the returned job/work ID.
5. Start a background review subagent and verify the parent turn continues.
6. Disconnect and reconnect before completion; verify the final state appears once.
7. Abort the session and verify both shell and subagent work stop.

## Open decisions

1. Should `work_id` be separate from `job_id`/`subagent_id`, or should each backend ID be the canonical work ID? Recommendation: separate common ID with backend ID in metadata; this prevents future backends from inheriting incompatible ID formats.
2. Should completion summaries be injected automatically into the next model turn or only exposed through a tool? Recommendation: compact automatic notification plus explicit detail retrieval.
3. What approval behavior is correct when a detached worker reaches an approval prompt while no client is watching? Recommendation: pause with a visible `needs_approval` state and expire after a bounded timeout; never silently approve.
4. Should background subagents be persisted as full sessions? Recommendation: keep this feature ephemeral-but-recoverable first; use the existing orchestration/session-spawn plan if the user needs durable, independently resumable agent sessions.
5. What event retention window is sufficient for mobile reconnects? Start with the existing session/event retention policy and make terminal work records available through the detail endpoint.
6. Should a finished background operation wake an idle server-side parent loop? Not in v1; notify the next turn and clients without interrupting an active provider request.

## Relationship to existing plans

- `docs/plan/orchestration.md` describes durable Level 2 session spawning. This plan implements the lower-level asynchronous work callback mechanism and must not accidentally turn ephemeral subagents into durable sessions.
- `docs/plan/composer-context-attachments.md` may share output cursor and artifact handling; reuse its bounded attachment conventions rather than creating a second output transport.
- Existing subagent lifecycle protocol and UI work should be extended additively, not replaced.

## Definition of done

The feature is complete when a Console agent can start either a background shell or background subagent, continue the current interaction, receive exactly one durable completion callback, inspect the complete result later, and cancel the work safely. The behavior must survive client reconnects, preserve permission and session ownership boundaries, remain compatible with synchronous tools and older clients, and be covered by targeted server, protocol, and client tests.
