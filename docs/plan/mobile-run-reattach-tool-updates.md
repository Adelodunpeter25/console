# Android Mobile Run Reattach and Tool-Call Update Plan

## Problem

When an agent run is active and the Android app is closed and reopened, plain streamed text can resume, but tool-call activity and tool results may not appear until the user leaves the session and opens it again.

The intended behavior is:

1. The Android app starts or reopens a session.
2. It loads the persisted conversation.
3. If the server still reports an active run, it immediately attaches to that run.
4. Replayed and future SSE events update the same visible run activity, including tool calls, tool results, permissions, questions, todos, and completion.

This plan is Android-only. iOS, web, and fallback stream implementations are intentionally out of scope.

## Scope

### In scope

- `packages/utils/src/run-hub.ts` replay and subscriber behavior.
- Server active-run status and attach-stream behavior.
- Android native SSE sequence handling.
- Mobile session hydration and automatic reattach.
- Rebuilding tool-call activity after a cold app restart.
- Stable run identity and replay ordering.
- Android lifecycle handling when the app returns from the background or is relaunched.

### Out of scope

- iOS/Web XHR fallback behavior.
- Interactive terminal tabs, PTYs, terminal WebSockets, or terminal backgrounding.
- Changing the `RunEventHub` into a general event bus.
- Persisting an active run across a server restart.
- New notification UX beyond what is needed to expose an active run or attention state.

## Relevant Current Implementation

- `packages/utils/src/run-hub.ts` assigns monotonic sequence numbers, keeps a bounded replay buffer, coalesces consecutive text/thinking deltas, and preserves tool-call frames as discrete events.
- `apps/server/api/src/services/run.service.ts` owns active runs and one `RunEventHub` per session. It persists model turns and tool results while broadcasting events to subscribers.
- `apps/server/api/src/routes/run.ts` exposes `GET /api/sessions/:id/run/stream?since=<seq>` and sends `streamReset` before replay when `since` is supplied.
- `apps/mobile/stores/chat/run-stream-controller.ts` reconnects with the last received sequence and attaches with `since=0` after a cold restart.
- `apps/mobile/hooks/useChatStream.ts` loads session messages and calls `attachServerRun()` only when the local state is not running, the local controller is absent, and the separate status store says `working`.
- `apps/mobile/stores/useChatStore.ts` applies events to local session state. Tool calls are created from `modelStreamEnd`, tracked by `toolExecutionStart`, and completed by `toolExecutionResult`/`toolExecutionEnd`.
- `apps/mobile/utils/chat-events.ts` updates the latest run by array position and `streamReset` currently clears only streaming text and thinking buffers.
- `apps/mobile/modules/native-stream/index.ts` provides the Android native stream path and forwards SSE sequence numbers from native events.

## Diagnosis to Verify

There are two likely failure points that should be instrumented and tested separately:

1. **Attach is skipped after cold start.** The session status store may not yet contain `working` when `useChatStream()` runs. The chat screen then loads persisted data but never calls `attachServerRun()`. Returning to the session list causes another fetch cycle, which accidentally makes the status available.
2. **Tool replay is applied to an incomplete local run.** The replay resets text buffers but not `activeToolCalls` or the current run timeline. Tool events are applied through `updateLatestRun()`, which assumes the last local run is the server run. Persisted messages and replayed events can therefore land in different run states.

Add temporary diagnostic logging or test hooks before changing behavior. Capture session ID, server status, local `running`, controller presence, attach decision, replay sequence numbers, replay event types, current run ID, and event counts.

## Design

### 1. Make Android cold-start hydration explicit

Create one Android-safe session hydration/reattach flow rather than relying on unrelated query effects:

1. Fetch the session detail and latest persisted messages.
2. Seed the session header/status store from the response.
3. Hydrate local messages and reconstruct persisted runs.
4. Query the server's active-run state or use the authoritative `working` header state.
5. If a run is active and no controller exists, create the controller and attach with `since=0`.
6. Mark the local session as running before replay events are applied.
7. Keep the stream attached until a terminal `done`/`aborted` frame or a confirmed `409`.

The routine must be safe to call repeatedly. It must not create duplicate controllers or duplicate subscriptions.

The app must run this flow when the chat session is first selected after launch and when Android returns to the foreground. Do not require navigating to Home and back.

### 2. Use the authoritative server status

Avoid making attachment depend solely on `sessionStatuses$[sessionId].peek()` being initialized by a separate session-list request.

Prefer one of:

- Include the active-run state in the session detail response and use it directly.
- Fetch the session detail before making the attach decision and update the status store from that response.
- Add a small active-run status endpoint if the existing header status cannot be trusted during startup.

Keep the server status and local runtime state distinct:

- Server `working` means an active run may exist and must be checked/attached.
- Local `running` means the Android UI currently owns an active stream.
- Controller presence means a local stream is being managed.

### 3. Make replay reset complete and deterministic

When the attach stream sends `streamReset`, clear transient state that belongs to the replayed run:

- `streamingText`.
- `streamingThinking`.
- `activeToolCalls`.
- Pending transient interaction state only if the replay will resend it.

Do not blindly delete persisted messages or completed historical runs.

The preferred approach is to create a replay target for the current server run, apply all replayed events to that target, and then merge it with persisted messages. If a smaller change is required, identify the latest persisted working run explicitly and clear/replace only that run's transient events before replay.

### 4. Stop selecting runs by array position

Add a stable server run identifier to the active-run stream context and relevant events, or maintain a reliable session-to-run association during attach.

Use that identifier when applying:

- `modelStreamEnd`.
- `toolExecutionStart`.
- `toolExecutionResult`.
- `toolExecutionEnd`.
- `sessionEnd` and terminal frames.

Until a server run ID is available, the controller should create and retain one local replay run ID for the attach lifecycle rather than allowing every handler to assume `runs[runs.length - 1]` is correct.

Tool-call IDs remain the key for matching individual results, but they are not sufficient to identify the owning run across a cold restart.

### 5. Verify replay completeness and ordering

For an attach with `since=0`, the server must replay the buffered events in sequence order. A tool-call replay should contain the necessary lifecycle events, for example:

```text
streamReset
modelStreamEnd (turn contains tool call)
toolExecutionStart
toolExecutionResult
toolExecutionEnd
modelStreamEnd (next turn)
...
done
```

The hub should continue to:

- Preserve discrete tool frames without coalescing them.
- Deliver replay events before live events for the newly attached subscriber.
- Keep sequence numbers monotonic.
- Ensure a subscriber does not receive a frame twice or miss a frame at the replay/live boundary.

Add a diagnostic assertion or test helper that verifies replay sequence monotonicity and expected event order.

### 6. Handle Android stream lifecycle correctly

The Android native stream is the only supported mobile transport for this feature.

When the app is backgrounded or the process is killed:

- Do not interpret the lost native stream as a user abort.
- On app foreground or relaunch, create a new stream ID and attach using the last known sequence when available; use `since=0` for a cold start with no local sequence.
- Treat a `409` as “the run has already settled,” then refresh persisted session data before finalizing local state.
- Ignore late events from an old stream ID/controller.
- Ensure native event subscriptions are removed when replacing or cancelling a stream.

No iOS/Web fallback changes are required or desired.

### 7. Reconcile persisted data after completion

The server persists model turns and tool results while the run executes. After receiving `done` or `409`:

1. Finalize the local run once.
2. Refresh the session header/status.
3. Refetch the latest session messages if needed to reconcile replay state with storage.
4. Preserve the visible tool activity rather than replacing it with text-only state.

Do not use a Home-screen round trip as the reconciliation mechanism.

## Implementation Steps

1. Add Android-only diagnostics around cold-start attach decisions and replay events.
2. Add focused tests for `RunEventHub` replay ordering and tool-call frames.
3. Refactor mobile session hydration so status seeding, message hydration, and attach decision happen in one deterministic path.
4. Trigger the hydration/reattach flow on chat entry and Android foreground resume.
5. Update `streamReset` handling to clear transient tool state without deleting persisted history.
6. Introduce stable run identity, or a controller-owned replay run association, and route tool events through it.
7. Verify Android native SSE sequence handling and old-controller cleanup.
8. Reconcile persisted messages/status after terminal frames and `409` responses.
9. Update comments in `run-hub.ts`, the run controller, and the mobile hydration code so the ownership and replay contract are explicit.

## Verification

### Unit and integration tests

- `RunEventHub` replays discrete `modelStreamEnd`, `toolExecutionStart`, `toolExecutionResult`, and `toolExecutionEnd` frames in sequence order.
- A subscriber attached with `since=0` receives the full buffered active run and then live events without duplicates.
- A subscriber attached with a later sequence receives only newer events.
- Coalesced text replay does not coalesce or discard tool-call frames.
- `streamReset` clears transient text, thinking, and active tool state while preserving persisted historical messages.
- Tool results match the correct tool call after replay.
- A stale controller cannot update state after a new controller attaches.
- A `409` causes a persisted-data refresh and exactly one local finalization.

### Android manual checks

1. Start a run that produces text followed by a tool call.
2. Close or force-stop the Android app while the tool is running.
3. Reopen directly into the same session.
4. Confirm the app attaches without visiting the session list.
5. Confirm the tool call appears while it is active.
6. Confirm the tool result appears when it completes.
7. Confirm subsequent model text continues in the same run.
8. Repeat while a permission request or question is pending.
9. Repeat after the run has completed while the app was closed; confirm the persisted tool call/result is visible immediately.
10. Background and foreground the app during streaming; confirm only one stream is active and no duplicate tool events appear.
11. Force a network interruption and confirm Android reconnects using the last sequence.
12. Confirm the run remains correctly marked completed, failed, aborted, or needs attention.

## Success Criteria

- Reopening Android directly into a session automatically reconnects to an active server run.
- Tool calls, tool results, permissions, questions, todos, and final text update without leaving and re-entering the session.
- No duplicate tool calls or results appear after replay.
- No active run is incorrectly marked aborted merely because Android lost its stream.
- The server remains the source of truth for active-run status and persisted messages.
- The behavior does not modify interactive terminal support or any non-Android stream fallback.
