# Plan Mode — Design & Implementation

## 1. Overview

Plan mode is a lightweight workflow, not a state machine: the agent researches a task read-only, writes a plan artifact under a controlled directory, asks the user to approve it via the question tool, and — once approved — the session escalates to bypass permissions so the agent can implement. There is no verification phase; the agent (or AGENTS.md) handles verification during execution. When finished, the agent marks the plan `done`.

## 2. Flow

```
user sends task while approvalMode = plan-mode
  |
  v
EXPLORING - read-only research; writes allowed ONLY under <cwd>/.agents/plans/**
  |
  v
agent writes plan artifact + asks via `ask` tool with options ["proceed", "reject"]
  |
  v
user answers (option or custom text)
  |-- answer === "proceed" -> session escalates to full-access
  |                           (persisted in DB, applied mid-run)
  |
  |-- "reject" or custom feedback -> agent revises plan and re-asks
  |
  v
agent implements (full-access) and flips plan status frontmatter to done
```

## 3. Write policy during plan-mode

- Read-tier tools: allowed (unchanged).
- Write-tier tools (`writeFile`, `editFile`, `batchWrite`): auto-allowed only when the resolved target path is inside `<cwd>/.agents/plans/`; anywhere else -> the existing `permissionRequest` prompt.
- `bash` (exec tier): prompts for permission, as today.
- `subagent` (read tier): allowed, but delegation must not escape plan-mode. When the parent's mode getter resolves to `plan-mode`, the spawned child loop runs with `plan-mode` + the same `plansDir` instead of the hardcoded `accept-edits`; reading the getter at spawn time means a mid-run escalation also propagates to later-spawned children. All other parent modes keep today's behavior.

## 4. Plan artifact

Location: `<cwd>/.agents/plans/<date>-<name>.md`

Frontmatter:

```yaml
---
plan_id: plan_2026_08_18_abc123
status: draft            # draft -> approved -> done
workspace: /path/to/repo
created_at: 2026-08-18T12:00:00Z
updated_at: 2026-08-18T12:00:00Z
---
```

Body: objective, scope/non-goals, files to change, implementation steps, risks, validation/test commands, open questions.

The agent updates `status` to `approved` immediately after receiving "proceed" (before any implementation step), and to `done` when the work is finished. `status` is advisory metadata: the harness neither enforces nor validates transitions.

## 5. Question tool

The existing `ask` tool already supports multiple-choice `options` plus free-text answers, and the desktop `QuestionPanel` renders options and an always-available custom text field (custom text takes precedence). No UI change needed. In plan mode the agent presents its plan with options `["proceed", "reject"]`.

## 6. Escalation on approval

Detected in the server's ask handler: when the current mode is `plan-mode`, the question options include `"proceed"`, and the answer is exactly `"proceed"`.

Escalates the session to `full-access` (bypass permissions):

- The running agent's approval mode is switched immediately (mid-run), so subsequent tool calls no longer prompt.
- The session's persisted `approval_mode` is updated so later runs in the session stay in `full-access` until the user switches mode.

`"reject"` or any custom answer does not escalate; the agent receives the text and revises the plan.

## 7. Completion

When implementation is finished the agent edits the plan file and sets `status: done` in the frontmatter. No further harness involvement.

## 8. File changes

- `packages/types/src/plan.ts` (new) — `PlanFrontmatter` type; export from `packages/types/src/index.ts`.
- `packages/types/src/events.ts` — add `planApproved` event so clients can observe the escalation during a live run.
- `apps/server/agent/src/permissions/approval.ts` — path-aware plan-mode write policy (`resolveApproval` gains a `plansDir` option).
- `apps/server/agent/src/service/types.ts` — `AgentLoopConfig.approvalMode` accepts `ApprovalMode | (() => ApprovalMode)`; add `plansDir`.
- `apps/server/agent/src/service/tool-executor.ts` — resolve the mode getter per call; pass `plansDir` into `resolveApproval`.
- `apps/server/agent/src/service/agent.ts` — `run()` passes an `approvalMode` getter so mid-run `setApprovalMode` takes effect.
- `apps/server/agent/src/tools/subagent.ts` — accept the parent's `approvalMode` getter + `plansDir`; when the parent resolves to `plan-mode`, spawn child loops with `plan-mode` + `plansDir` (replacing the hardcoded `accept-edits` in that case) so delegation cannot bypass the write policy.
- `apps/server/api/src/services/run.service.ts` — compute `plansDir`, wire the getter + `plansDir`, escalate in the ask handler, emit `planApproved`.
- `apps/server/agent/src/systemprompt/builder.ts` — rewrite plan-mode instructions (plan-dir writes, frontmatter convention, proceed/reject question, mark `approved` on proceed, mark `done` on completion).

## 9. Tests

- Extend `apps/server/tests/permissions.test.ts`: plan-mode write inside `plansDir` -> allow; outside -> prompt; `batchWrite` all-inside -> allow / any-outside -> prompt.
- New `apps/server/tests/plan-mode-flow.test.ts`: mode getter escalation — `ask` handler resolving `"proceed"` switches to `full-access` and a subsequent write outside `plansDir` runs without a permission prompt; `"reject"`/custom answers do not escalate.
- Subagent containment (in `plan-mode-flow.test.ts`): with the parent in plan-mode, a spawned child attempting `writeFile` outside `plansDir` gets a permission prompt, not a silent allow; after escalation the same spawn runs `accept-edits`.