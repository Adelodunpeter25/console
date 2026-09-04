# Plan Mode Permission Architecture Plan

## 1. Objective
Enable **Plan Mode** to operate with full autonomous exploration and verification capabilities (reading files, directory tree inspection, executing non-destructive read/diagnostic commands, using subagents) without nagging the user with permission popups, while strictly safeguarding the workspace code from modifications until the user explicitly approves and executes the plan.

---

## 2. The Problem with Current Plan Mode
Right now, in `apps/server/agent/src/permissions/approval.ts`:
```typescript
if (mode === "plan-mode") {
  if (tier === "read") {
    return { policy: "allow", tier };
  }
  return {
    policy: "prompt",
    tier,
    reason: `Tool '${tool.name}' requires upgraded permission because Plan Mode is read-only.`,
  };
}
```
- Any tool categorized as `exec` (like `execute_command`) or `write` (like `write_to_file`, `replace_file_content`) immediately halts the agent and forces a permission prompt modal in the UI.
- In reality, an agent constructing an accurate plan *needs* to run `git status`, `git diff`, `bun pm ls`, `cargo tree`, `find`, or test dry-runs to inspect the environment.
- Forcing prompts for basic inspection commands interrupts the agent and defeats the purpose of autonomous planning.

---

## 3. Architecture & Design

```
┌─────────────────────────────────────────────────────────────┐
│                       PLAN MODE                              │
├──────────────────────────────┬──────────────────────────────┤
│  AUTONOMOUS (Allow)          │  GATED / RESTRICTED (Prompt) │
│  - All read & search tools   │  - Workspace file edits      │
│  - Inspection bash commands  │  - Destructive bash commands │
│  - Ephemeral subagents       │  - Code mutations            │
│  - Read-only diagnostics     │                              │
└──────────────────────────────┴──────────────────────────────┘
```

### A. Intent & System Prompt Framing
In the system prompt for Plan Mode:
> *"You are in PLAN MODE. Your objective is exclusively to research, analyze, and formulate a clear, step-by-step implementation plan. You may freely inspect the codebase, run non-destructive diagnostic/read commands, and utilize subagents. You must NEVER modify codebase files or execute destructive actions. Present your final output as a structured plan for the user to review."*

### B. Intelligent Command Policy for `execute_command`
Instead of treating all bash commands as `exec` requiring prompts:
1. **Safe Diagnostic Commands (Auto-allowed in Plan Mode):**
   - `git status`, `git diff`, `git log`, `git branch`
   - `ls`, `cat`, `head`, `tail`, `grep`, `rg`, `find`
   - `cargo check`, `cargo tree`, `bun run check`, `npm test` (read/typecheck runs)
2. **Mutating / Destructive Commands (Prompt/Block):**
   - File modifying commands (`rm`, `mv`, `sed -i`, `git checkout .`, `git reset --hard`)
   - Direct file edits via tools (`edit_file`, `write_to_file`)

### C. Subagent Autonomy in Plan Mode
Subagents spawned during Plan Mode inherit `approvalMode: "full-access"`, allowing them to conduct deep codebase investigations without interrupting the user.

---

## 4. Implementation Steps
1. **Enhance `apps/server/agent/src/permissions/approval.ts`:**
   - Refine `plan-mode` policy resolution: allow non-destructive commands and inspection tools automatically.
   - Restrict project file modifications to `prompt` with a clear explanation: *"Plan Mode cannot modify workspace files directly. Proceed with the plan to apply changes."*
2. **Update System Prompt (`plan-mode`):**
   - Emphasize investigation and artifact creation over direct code application.
3. **Verification:**
   - Run server test suite `bun tests/approval.test.ts`.
