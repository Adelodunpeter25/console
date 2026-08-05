# Agent Activity Timeline

## Objective

Build a session-scoped activity timeline that shows what the agent is doing immediately after the user's prompt. The timeline should update live while the run is active, then collapse into a duration summary when the run completes. Expanding the summary should reveal the useful progress messages, tool calls, results, permissions, questions, todo updates, errors, and abort state associated with that run.

The activity timeline should describe observable agent progress without exposing private hidden reasoning. It may show model-authored progress text and structured lifecycle events emitted by the harness.

## Phase 1: Define the activity model

- [ ] Define a shared `RunActivityEvent` type for observable progress entries.
- [ ] Support assistant progress text, tool calls, tool results, permissions, questions, todo updates, errors, and run state changes.
- [ ] Add stable event IDs and timestamps so entries can be updated in place as tools move from running to completed or failed.
- [ ] Define explicit run states: `working`, `needs_attention`, `completed`, `aborted`, and `failed`.
- [ ] Decide which events are transient UI state and which events must be persisted for reloads.

## Phase 2: Emit a complete event timeline from the harness

- [ ] Emit a run-start event immediately when an agent run begins.
- [ ] Convert assistant turns that lead into tool use into observable progress entries.
- [ ] Keep existing tool start, result, and end events linked to the activity event for the same tool call.
- [ ] Emit permission and question events with their pending/resolved state.
- [ ] Emit todo-created and todo-updated events with the complete session todo list.
- [ ] Emit run completion, abort, and failure events even when the provider or client disconnects.
- [ ] Ensure events are tagged with the session ID and run ID so activity cannot bleed between chats.

## Phase 3: Store activity per chat session

- [ ] Add a dedicated activity state to the desktop chat session store.
- [ ] Start a fresh activity timeline for every new prompt.
- [ ] Merge streamed events incrementally without replacing activity from another session.
- [ ] Update existing entries when a tool result arrives instead of creating duplicate rows.
- [ ] Preserve the completed timeline after the run ends so it can be expanded.
- [ ] Clear only transient active state when aborting; preserve the observable work already performed.

## Phase 4: Build the activity timeline UI

- [ ] Render the activity block immediately after the latest user prompt.
- [ ] Show a live duration and working state as soon as the prompt is submitted.
- [ ] Render assistant progress messages in chronological order.
- [ ] Render tool calls and results using the existing tool result and syntax-highlighting components.
- [ ] Render permission requests, questions, todo updates, and errors as timeline entries.
- [ ] Keep the timeline expanded while the agent is working.
- [ ] Collapse the timeline by default when the run completes.
- [ ] Show a compact `Worked for …` summary row when collapsed.
- [ ] Allow expanding the completed summary to inspect the full run.
- [ ] Keep the final assistant response visually separate from the activity timeline.
- [ ] Preserve the current grouped-tool behavior: repeated calls group together and child rows remain collapsed by default.

## Phase 5: Persistence and recovery

- [ ] Persist the completed activity timeline or a reconstructable activity record with the session.
- [ ] Restore completed activity when switching chats or reopening the desktop app.
- [ ] Restore tool completion states correctly after an interrupted run.
- [ ] Ensure a stopped agent can view the current session todo list on the next prompt, including completed and unfinished items.
- [ ] Handle server restart and client reconnect without mixing activity across sessions.
- [ ] Add bounded retention or compaction for very long activity timelines.

## Phase 6: Reliability and edge cases

- [ ] Verify that runs with no tool calls still show a duration summary.
- [ ] Verify that a run with several tool batches maintains chronological ordering.
- [ ] Verify that a failed or denied permission is shown as a decision state, not as a generic tool failure.
- [ ] Verify that aborting one session does not stop or alter another session's activity.
- [ ] Verify that late SSE events cannot mutate a newer run in the same session.
- [ ] Verify that duplicate events are idempotent.
- [ ] Verify that completed tool calls never return to a spinner on later renders.
- [ ] Verify that todo panels disappear when all todos are completed.

## Phase 7: Verification and cleanup

- [ ] Add unit tests for activity event reduction and event ordering.
- [ ] Add tests for session isolation and run isolation.
- [ ] Add tests for tool lifecycle transitions: started, completed, failed, denied, and aborted.
- [ ] Add tests for todo activity updates and resume-after-stop behavior.
- [ ] Add desktop component tests for live, collapsed, expanded, and empty activity states.
- [ ] Run focused desktop typecheck and backend tests.
- [ ] Run Tauri/Rust checks when event model changes affect the bridge.
- [ ] Review the UI for excessive vertical space, duplicate tool output, and auto-scroll regressions.
- [ ] Commit each completed implementation task with a concise single-line commit message.
