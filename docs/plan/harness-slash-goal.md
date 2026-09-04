# Autonomous `/goal` Harness Implementation Plan

## Overview
This plan specifies the architecture and phased implementation for adding the autonomous `/goal` command to the agent harness (mirroring `oh-my-pi` patterns in `apps/server/agent`).

`/goal` transforms the standard interactive conversational agent turn into an **autonomous, long-horizon supervisory loop**. It executes continuously until the agent emits `<!-- GOAL_COMPLETE -->` or is cancelled by the user (`<!-- GOAL_CANCELLED -->` / abort signal).

---

## Key Requirements & Design Decisions
1. **Automatic Permission Bypass**:
   - `/goal` runs autonomously without interactive prompts. When triggered, the session approval mode automatically elevates to `full-access` (bypassing manual confirmation for tools, file writes, and bash execution) for the duration of the goal run.
2. **Autonomous Supervisory Turn Loop**:
   - Standard turns end when the model finishes its tool execution.
   - Under `/goal`, if the model stops without emitting `<!-- GOAL_COMPLETE -->`, the harness automatically prompts the agent to continue executing towards the objective.
3. **Context Window & Compaction Resiliency**:
   - Long goals spanning dozens of iterations actively leverage existing auto-compaction and tool result truncation (`DEFAULT_TOOL_RESULT_MAX_CHARS`) to stay within model token limits without context overflow.
4. **Safety & Circuit Breakers**:
   - Iteration budget cap (configurable max turns, e.g., 50–100 iterations).
   - Responsive user cancellation (`SIGINT`, abort controllers, UI Stop button).
   - Loop detection (guards against repetitive idempotent failures).
5. **No External Evaluation Gate**:
   - The harness relies strictly on the LLM's goal completion signals rather than mandatory external test/lint runners.

---

## Architecture Flow

```
┌────────────────────────────────────────────────────────────────────────┐
│                        User triggers `/goal <task>`                    │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│ 1. Parse Slash Command & Elevate Approval Mode to `full-access`        │
│ 2. Inject Goal Supervisory System Directives                           │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼
                      ┌───────────────────────────┐
                      │    Execute Agent Turn     │ ◄────────────────────┐
                      │ (Model + Tool Execution)  │                      │
                      └─────────────┬─────────────┘                      │
                                    │                                    │
                                    ▼                                    │
                 ┌──────────────────────────────────────┐                │
                 │ Auto-Compaction & Truncation Check   │                │
                 └──────────────────┬───────────────────┘                │
                                    │                                    │
                                    ▼                                    │
                   ┌──────────────────────────────────┐                  │
                   │ Is Goal Terminated or Cancelled? │                  │
                   └────────────────┬─────────────────┘                  │
                                    │                                    │
                  ┌─────────────────┴─────────────────┐                  │
                  │                                   │                  │
         Yes (`<!-- GOAL_COMPLETE -->`                │ No               │
              or `<!-- GOAL_CANCELLED -->`)           │ (Iter < MaxCap)  │
                  ▼                                   ▼                  │
┌───────────────────────────────────┐ ┌────────────────────────────────┐ │
│ Restore Original Approval Mode    │ │ Inject Continuation Prompt:    │─┘
│ Emit `goalEnd` Session Event      │ │ "Continue working towards goal"│
└───────────────────────────────────┘ └────────────────────────────────┘
```

---

## Implementation Phases

### Phase 1: Slash Command Parser & Approval Mode Override
- **Objective**: Recognize `/goal` from incoming prompts, extract the user objective, and temporarily elevate the session permission mode.
- **Tasks**:
  1. Add `/goal` command definition in `apps/server/agent/src/systemprompt/discover-commands.ts` and `apps/server/agent/src/commands/`.
  2. In `apps/server/api/src/services/session.service.ts` or agent dispatch handler:
     - Detect `/goal <objective>` prefix.
     - Save current `approvalMode` (e.g., `always-ask` or `accept-edits`).
     - Set session `approvalMode` to `full-access` for the active goal execution.
  3. Ensure tool executor allows all tiers (`read`, `write`, `exec`) without blocking on user approval.

---

### Phase 2: Autonomous Goal Execution Loop (`runGoalLoop`)
- **Objective**: Implement the continuous outer supervisor in `apps/server/agent/src/service/`.
- **Tasks**:
  1. Create `runGoalLoop` (or extend `runAgentLoop` in `apps/server/agent/src/service/agent-loop.ts`):
     - Track `iterationCount` against `maxIterations` (default: 50).
     - Inspect turn output for `<!-- GOAL_COMPLETE -->` and `<!-- GOAL_CANCELLED -->`.
  2. Implement continuation mechanism:
     - If the model yields a turn without the completion marker, synthesize a continuation turn:
       `"Continue working autonomously on the goal. Inspect your progress, take next steps, and output <!-- GOAL_COMPLETE --> when fully accomplished."`
  3. Emit dedicated events:
     - `goalStart`: `{ goal: string, maxIterations: number }`
     - `goalIteration`: `{ iteration: number, maxIterations: number }`
     - `goalComplete`: `{ totalIterations: number, reason: 'completed' | 'cancelled' | 'iteration_limit' }`

---

### Phase 3: Goal Prompting & Context Management
- **Objective**: Ensure the agent understands the goal contract and stays coherent across many turns.
- **Tasks**:
  1. Inject `/goal` system guidelines into the active turn:
     - Instruct model to work step-by-step and keep track of intermediate progress.
     - Clarify that it has full autonomy and should output `<!-- GOAL_COMPLETE -->` only when all deliverables are in place.
  2. Verify compaction behavior in `apps/server/agent/src/compaction/index.ts`:
     - Keep recent turns in full fidelity while summarizing older intermediate turns.
     - Retain active file diffs and tool summaries.

---

### Phase 4: Termination, Cleanup & UI Events
- **Objective**: Gracefully handle cancellation, iteration exhaustion, and restore session state.
- **Tasks**:
  1. Revert `approvalMode` back to the pre-goal setting when execution concludes.
  2. Handle abort signal immediately:
     - User clicking "Stop" in UI cancels the loop and emits `goalComplete` with `reason: 'cancelled'`.
  3. Stream goal progress events across WebSocket/SSE to desktop and mobile clients so the UI displays active goal status and iteration count.

---

## Verification Plan
1. **Unit Tests**:
   - `apps/server/tests/goal-harness.test.ts`:
     - Test that `/goal` elevates approval to `full-access`.
     - Test that multi-turn loop continues until `<!-- GOAL_COMPLETE -->` is emitted.
     - Test that reaching `maxIterations` stops execution gracefully.
     - Test that cancellation (`AbortSignal`) stops execution immediately.
2. **Integration Test**:
   - Dispatch `/goal create a helper file and export a function` and verify end-to-end multi-step tool calls without permission prompts.
