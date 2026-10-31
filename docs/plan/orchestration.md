# Orchestration — the `console` kernel

Replaces the `project_scripts` and `ports` agent tools with a single JavaScript kernel that gives the agent a control plane over the desktop app: run project scripts, manage ports, spawn sessions, inspect and steer them, and eventually drive the UI.

The precedent is already in this codebase. `computer` + `computer_reset` (`internal/services/cua/js_tools.go`) replaced 56 per-tool schemas — roughly 28K tokens re-sent on **every** turn — with two fixed two-field schemas, documenting the driver API as methods in the `/computer-use` skill so it reaches the model once. `console` applies that same move to the app's own control surface. Today `DefaultToolNames` (`internal/agent/systemprompt/builder.go:38`) still ships the `ports` and `project_scripts` schemas on every turn.

Target: `internal/services/console/` (kernel), `internal/agent/tools/console_tool.go`, `internal/run/turns.go` (binding), `internal/services/session/` (spawn surface).

---

## Status (as of 2026-10-30)

| Phase | State | Notes |
| :--- | :--- | :--- |
| 1 — Kernel + ports | **Done** | `console` / `console_reset` tools, `ports.list/forward/unforward`; `ports` and `project_scripts` removed from `DefaultToolNames` |
| 2 — Scripts | **Done** | `scripts.list/run/status/stop` plus `scripts.output` with absolute-offset cursors that survive the 256KB buffer trimming |
| 3 — Discovery + session control | **Done** | `models()`, `approvalModes()`, `sessions.list/read/delete`; deleting the active session is refused. Also shipped, not in the plan: `projects.list/add` |
| 4 — Spawn (`sessions.create`) | **Partial** | `sessions.create` takes prompt, model, provider, `thinking_level` (validated against the model), cwd, project, worktree, and `approval_mode`, which is **clamped to the calling run's mode** with the clamp reported. **Decision:** it stays `sessions.create`; the `console.spawn` rename is dropped. **Missing:** depth cap, token budget, concurrency cap, usage rollup |
| 5 — `sessions.fork` | **Done** | Exposed to the kernel; the fork's inherited approval mode is clamped to the calling run |
| 6 — Security prerequisites | **Done** | Path boundary shipped (`permissions/guard.go`). Plan mode is now enforced: `Resolve` denies write and exec tiers, and the only write it allows is the `writePlan` tool, confined to `.console/plans/` (§8.2) |
| 7 — GitHub | **Half done** | `GH_TOKEN` injection shipped (`providers/github/ghenv.go`). **Missing:** `console.pr.*`, GitHub REST service |

**The API as built** (every method takes one object, never positional values): `ports.{list,forward,unforward}`, `scripts.{list,run,status,output,stop}`, `models`, `approvalModes`, `projects.{list,add}`, `sessions.{list,read,create,delete,abort,set_model,set_approval,prompt,queue,clear_queue,fork}`. **Not in the API yet:** `console.pr.*`.

**Kernel behaviour (changed after the first round of agent testing):**
- **Plain results.** A call returns its data directly (`console.sessions.list()` is the array), not a `{content, structuredContent, isError}` wrapper; a failure throws with the reason. Only `console` does this; computer use keeps the wrapper because it returns images.
- **Strict arguments.** Every method lists the keys it accepts (`methodArgs` in `runtime.go`). An unknown key or a wrong type fails with the accepted names instead of being ignored, so `{projectId: ...}` is caught. Whole numbers are accepted from JavaScript as any integer type (a bug rejected every port before this).
- **Helpful errors.** An unknown method or namespace lists the valid ones; the timeout message says what to check (`sessions.list()`, `scripts.status()`, `ports.list()`); output past 100,000 characters is cut with a note saying how much was lost, and `sessions.read` takes `limit`/`before` to ask for less.
- **`sessions.prompt`** starts an idle session or queues the prompt for a busy one (`queued: true`). One prompt can wait per session and it refuses to replace one, including the user's own. `sessions.queue` shows the waiting prompt and `sessions.clear_queue` discards it. `sessions.abort` also clears it, so an aborted session does not restart itself from its queue. A turn's approval mode is capped at the caller's even if the target stores a higher one.
- **`sessions.create`** with a prompt reports `started: true` and `status: working`.

**Permission ceiling:** enforced for `sessions.create`, `set_approval` and `fork` (`effective = min(run mode, requested)`, reported as `clamped`). With no run behind the kernel the ceiling fails closed to `plan-mode`, and plan mode is now genuinely read-only, so the ceiling means what it says. A run cannot abort itself or change its own approval mode.

**Plan mode (enforced):** write and exec tools are denied, never prompted. Reads, `browser` and `memory` (now read tier) still run. `writePlan` is the one exception: it writes `.console/plans/<name>.md` only, builds the filename itself, refuses symlinks, and the folder ignores itself with its own `.gitignore`. Subagents inherit the mode. Known gaps, accepted: `browser` can still submit forms, and MCP tools are tiered by naming convention.

**Plan-mode bash.** `bash` runs under a read-only OS sandbox in plan mode only (`internal/agent/tools/sandbox*.go`): the kernel refuses every file write outside the system temp folder, so how a command is phrased does not matter. Network is open, reads are unrestricted (secrets included, by decision), `git push` works. macOS uses `sandbox-exec`; Linux uses `bwrap` (needs unprivileged user namespaces). A probe runs once and really tries a write; if it does not work, or the machine has no sandbox tool (or is Windows), plan-mode bash runs **unsandboxed** (the owner's decision), so on such a machine plan mode is not read-only for shell commands. `bashJob` stays denied in plan mode. Other modes are untouched. **Verified:** macOS, with the real sandbox, including a mutation check that disables it and watches the tests fail. **Not verified:** the Linux `bwrap` path, which compiles and type-checks but has not been run; run `go test ./tests/tools/ -run "Sandbox|PlanBash"` on the Linux box, and note it skips (rather than fails) if `bwrap` is missing.

**Capability registry (how `/commands` work).** `/console`, `/visualize` and `/computer-use` are declared once in a table (`internal/run/capabilities.go`); the run loop, the slash-command popup and `readSkill` all read it, so none can drift from the others. Which are switched on is stored in the session row (`sessions.capabilities`), so it survives a server restart (it used to live in memory). The agent cannot type a slash command, so `readSkill` knows them: `readSkill()` lists them with their state, and `readSkill("visualize")` or `("console")` returns the reference **and switches the capability on**, loading its tools into the running turn. `computer-use` is the exception: it needs the user's consent and OS permissions, so `readSkill("computer-use")` tells the agent to ask the user to type `/computer-use` and changes nothing. Adding a capability is one entry in the table.

**Tests** live beside the code in `internal/services/console/*_test.go` (ceiling, abort, set_model, set_approval, fork, thinking level, queue, strict arguments, error messages), not in `tests/tools/TestConsole*` as the phases below say. Script cursor tests: `tests/api/scripts_output_cursor_test.go`. Capability registry: `internal/run/capabilities_test.go`. Path-guard and plan-mode tests: `tests/permissions/`. Sandbox tests: `tests/tools/plan_sandbox_test.go`. GH token tests: `tests/tools/bash_gh_env_test.go`.

### Next

- Depth cap 2, per-spawn token budget, concurrency cap and usage rollup for spawned sessions (§7)
- `console.pr.*` plus the GitHub service (Phase 7). Now designed in full, with the desktop and Android UI, in [`pull-requests.md`](pull-requests.md); that plan supersedes the Phase 7 sketch below

---

## 0. Non-Goals

- **Not a general "do anything" tool.** The kernel is an API surface over existing services, not a scripting escape hatch over the machine. Anything reachable must also be reachable through a typed Go path.
- **No bypass of approval gating.** Kernel write operations are `TierExec`/`TierWrite` and resolve through the same `permissions.Resolve` path as every other tool. The kernel is a transport, not a permission bypass.
- **No prompt-pollution by default.** API methods are documented in a skill, not in schemas. Only the two-field tool definition ships per turn.
- **Not a replacement for the ephemeral `subagent` tool** (Level 0 below). They serve different purposes and both ship.
- No change to the desktop UI layer in v1. Driving panes/tabs is deferred until the session-control surface is proven.

---

## 1. What Already Exists (verified)

Most of this is parameterization, not new construction. Grounded inventory:

**Spawn knobs exist as a struct.** `CreateSessionOptions` (`internal/types/session.go:31`) is already exactly the tuple this plan needs:

```go
Cwd, ProjectID, ModelID, Provider, ApprovalMode, ThinkingLevel, Worktree{Branch, BaseBranch}
```

**Session lifecycle operations exist.** All on `session.Service`: `Create`, `Fork(sourceID, messageID)`, `SoftDelete`, `PermanentDelete`, `Restore`, `UpdateModel`, `UpdateThinkingLevel`, `UpdateApprovalMode`, `UpdateCwd`, `GetSubagents`, `AttachWorktree`.

**A working nested-agent runner exists.** `internal/agent/loop/subagent.go` — nested run, lifecycle events (`EventSubagentStart`/`Activity`/`End`), usage tracking via `UsageTracker`, recursion blocked by stripping `subagent` from the nested tool set. Its input is only `{prompt, name, role}`; model, provider, tools, setup and approval mode are all hardcoded from `SubagentContext` in `internal/run/turns.go:378`.

**Kernel binding pattern exists.** `computerTool` embeds the declarative `tools.Tool` (for name/description/schema) and overrides `Execute` to bind one session's kernel — `internal/services/cua/js_tools.go:44`. `console` mirrors it.

**Port ownership is already modeled.** `PortOwner{Kind, ID}` plus `ObserveOutput` / `RemoveOwner`, with script runs registering themselves (`project_scripts_service.go:295`). Preserve this exactly; do not reimplement.

---

## 2. Design Principles

1. **Organize around sessions, not features.** Scripts, ports, and spawned tasks are not separate subsystems — they are things a *session owns*. A session already has `(model, provider, cwd, approval mode, thinking level)`, which is precisely `CreateSessionOptions`. Make that tuple the spine of the API rather than parallel `sessions.*` / `scripts.*` / `ports.*` namespaces.
2. **Reuse, never reimplement.** The kernel wraps the same services the existing tools call. `console.scripts.run` must produce identical behavior to `project_scripts` `start`, including worktree scoping (`resolveScriptRoot`) and port-owner cleanup on exit.
3. **The permission ceiling is inviolable.** See §4.
4. **Make the API discoverable, not guessable.** See §5.
5. **Cheap per turn, rich per call.** One small schema on every turn; the full API is loaded once via a skill, on demand.
6. **Isolated work means worktrees, not raw `cwd`.** See §6.
7. **Fail loud and specifically.** An unknown model ID names the valid ones; a dead session names its ID and status.

---

## 3. API Surface (proposed)

```js
// Discovery — cheap, read-tier, always safe (§5)
console.models()                        // ids, providers, supported thinking levels
console.approvalModes()
console.sessions.list({ status, project_id })

// Delegation (§4)
console.spawn({
  prompt, title,
  model, provider, thinking_level,
  approval_mode,                       // clamped to parent — result reports the clamp
  worktree: { branch, base_branch },   // isolation via worktree, not raw cwd
}) -> { session_id }

// Session control
console.sessions.read(session_id)      // transcript / status / result
console.sessions.fork(session_id, message_id)
console.sessions.delete(session_id)    // soft by default; never self; gated
console.sessions.set_model(session_id, model, provider)
console.sessions.set_approval(session_id, mode)   // clamped to the *owning* run
console.sessions.abort(session_id)

// Scripts + ports — same impl as the tools being replaced
console.scripts.list()
console.scripts.run(id, { wait_ms })
console.scripts.status(run_id)
console.scripts.stop(run_id)
console.ports.list()
console.ports.forward(port)
console.ports.unforward(port)

// Pull requests — typed wrappers over the stored GitHub PAT (§Phase 7)
console.pr.create({ owner, repo, head, base, title, body, draft }) -> { number, url }
console.pr.read({ owner, repo, number })     // status, checks, review state
console.pr.list({ owner, repo, state })      // open by default
console.pr.update({ owner, repo, number, title?, body?, base? })
console.pr.merge({ owner, repo, number, method })  // gated, never the default path
```

Deferred to a later phase: `console.tabs.*`, `console.panes.*`, `console.mcp.load(group)`, `console.ui.notify/openDiff/focusPane`.

---

## 4. The Delegation Ladder

The single most important distinction in this plan. Three levels, and only the third is what "spawn a task" usually means:

| Level | Shape | Persists? | User-visible? |
| :--- | :--- | :--- | :--- |
| **0 — today** | `subagent(prompt, name, role)` — inherits model, provider, cwd, tools, mode | No | No |
| **1 — knobbed** | ephemeral, but model/provider/thinking/mode overridable | No | No |
| **2 — real task** | `console.spawn()` → creates an actual `Session` | Yes | **Yes** |

Level 0 and 1 agents persist nothing — the `subagent.go` header says so explicitly, and they are assigned an in-memory `randomSubSession()` id. They vanish when the run ends. That is fine for "investigate this function and report back."

Level 2 is a different product surface: the session lands in the sidebar, can be opened, resumed, forked, and audited. **Anything the agent delegates that the user might want to inspect later must be Level 2.** Ephemeral delegation should never be the default for long-running work.

### The permission ceiling

The existing invariant in `subagent.go:35-38` is explicit and must survive:

> *"A subagent inherits its parent's tools, so it must inherit the parent's authority too: it may not act with more permission than the run that spawned it."*

Exposing `approval_mode` as a parameter breaks that invariant unless it is enforced. Rule:

```
effective = min(parent_mode, requested_mode)
```

with an ordering of `plan-mode < always-ask < accept-edits < full-access`. An `always-ask` parent can never produce a `full-access` child, no matter what it requests.

**Clamp silently, then report the clamp in the result.** If the clamp is silent the model retries the identical call several times, burning turns. The result should read: `requested full-access, clamped to accept-edits (parent ceiling)`.

The same ceiling applies to `console.sessions.set_approval`: a session may only be moved *down* from the ceiling of the run invoking it, never up.

---

## 5. Discovery Surface

Models fail in expensive ways when they guess. `console.models()` returns valid ids, providers, and per-model `SupportedThinkingLevels` (`internal/types/model.go:15`); `console.approvalModes()` returns the valid modes and their ordering. This converts "invalid model id" and "unknown approval mode" from mid-turn failures into knowledge the agent has before it acts.

Read-tier, no arguments, trivially cheap. Highest ratio of capability to cost in the whole plan.

---

## 6. Why `cwd` Is Deferred Behind Worktrees

Arbitrary per-agent `cwd` is the one requested capability that is **not** cheap, for a structural reason:

- `read_file`, `write_file`, `list_dir`, `glob`, `grep` are package-level `var` singletons (`internal/agent/tools/file_tools.go:56`+) built by `DefaultTools()` with nothing injected.
- They resolve paths straight into `os.Stat` / `os.ReadFile`. `read_file` has no cwd resolution at all.
- The server never calls `os.Chdir` (only `internal/bench/runner.go` does), so there is no implicit boundary.
- `bash` accepts an arbitrary `cwd` argument with no validation (`bash_tools.go:61-68`).

So `session.cwd` is today mostly *prompt metadata* plus script scoping — it does not bind the file tools. Making per-agent cwd real means converting the file tools from singletons into per-run constructed instances bound to a root. That is a real refactor with a real blast radius, and it should be its own project.

**v1 recommendation:** honor isolation only through `worktree`. `CreateWorktreeSpec{Branch, BaseBranch}` and `AttachWorktree` are already implemented. A spawned task gets a genuine directory, a genuine session, and no chance of two agents stomping each other's files in an overlapping tree.

**Caveat to verify before scoping:** the desktop may always launch the server with the project root as its process cwd, which would make the practical damage smaller than the code implies. Check this before sizing the refactor.

---

## 7. Cost and Recursion Guards

`subagent` currently cannot recurse — the nested tool set strips it (`subagent.go:143`). Once a session can spawn sessions that can spawn agents, unbounded fan-out becomes possible. Required:

- **Depth cap** — 2. Deeper nesting buys nothing a well-written Level 2 prompt doesn't.
- **Per-spawn token budget**, enforced and reported in the spawn result.
- **Concurrency cap** on live spawned sessions.
- **Usage rollup** — `UsageTracker` already collects subagent usage; spawned sessions should surface in the parent's total so the run's cost is attributable.

---

## 8. Prerequisites — Security Findings

These are load-bearing for this plan and were found while designing it. Both should be fixed before, or alongside, the kernel.

### 8.1 There is no filesystem boundary

A session created in `~/Developer/Projects/test` can **edit files anywhere on disk**, including `~/Developer/Projects/real-project`, if the model is told to. The chain:

- `permissions.Resolve(mode, tier)` (`permissions/permissions.go:42`) takes only a mode and a tier. No path, no cwd, not even the tool arguments.
- `Executor.Execute` (`loop/executor.go:39-106`) resolves permission then executes, inspecting nothing about the arguments in between.
- The file tools are unguarded singletons; `write_file` also creates parent directories automatically.
- `bash` accepts any `cwd`.

Per-mode effect — the only control anywhere is the tier-based approval dialog:

| Mode | Write outside the project dir |
| :--- | :--- |
| `always-ask` | Allowed, but prompts with the path visible |
| `accept-edits` | Allowed silently |
| `full-access` | Allowed silently |
| `plan-mode` | Allowed silently ← see 8.2 |

### 8.2 Plan mode's read-only guarantee is prompt-only

`permissions.go:44` treats `PlanMode` as `FullAccess` for all tiers. The code comment concedes it: the restriction on writes and exec "is communicated to the model only via the system prompt instructions, not enforced as a hard deny here."

A control that reads to the user as a guarantee but is only a suggestion to the model. `Resolve` should hard-deny `TierWrite` and `TierExec` in plan mode — a small change that converts a promise into an enforced rule.

### 8.3 Boundary design for the kernel

The boundary should be **path-based inside the kernel**, not tier-based, because spawning makes the mismatch dangerous: a `full-access` parent can spawn a Level 2 session on another project with no record beyond the sidebar entry. Concretely:

- Writes allowed only within the session's `cwd + extraDirs`; reads within a separate, wider read allowlist.
- `console.spawn` records the target root on the session so the audit trail shows where it was allowed to write.
- Symlink escapes must be resolved before the check, or the boundary is trivially bypassed.

---

## 9. Phased Implementation

### Phase 1 — Kernel skeleton + `ports` (smallest slice) — DONE
- `internal/services/console/`: manager, per-session runtime, `console` + `console_reset` tools bound via the `computerTool` pattern.
- Port methods wrapping `PortRegistry` unchanged. Remove `ports` from `DefaultToolNames`.
- `console-control` skill carrying the API documentation.
- **Verification**: `cd apps/server-go && go test ./tests/tools/ -run TestConsoleKernel -v` — kernel state persists across calls, `console_reset` clears it, variables do not leak between sessions.

### Phase 2 — `project_scripts` — DONE
- Script methods over `ProjectScriptsService`, preserving worktree scoping and port-owner cleanup exactly.
- Add the cursor pagination the service lacks (`capOutput` retains only the last 256KB with no offset), shared with `shell.read` from the attachments plan.
- Remove `project_scripts` from `DefaultToolNames`.
- **Verification**: `cd apps/server-go && go test ./tests/tools/ -run TestConsoleScripts -v` — behavior identical to the removed tool; cursor round-trips.

### Phase 3 — Discovery + session control — DONE
- `console.models()`, `console.approvalModes()`, `console.sessions.{list,read,delete}`.
- Delete: soft by default, permanent gated, **never the currently-running session**, and confirm when the session owns a worktree.

### Phase 4 — Spawn via `sessions.create` (Level 2) — PARTIAL (ceiling done; depth/budget/concurrency caps open)
- Session creation through the existing `session.Service`, worktree-scoped.
- Permission ceiling + clamp reporting (§4), depth cap, token budget, concurrency cap.
- **Verification**: `cd apps/server-go && go test ./tests/tools/ -run TestConsoleSpawn -v` — clamp enforced in both directions; budget and depth caps hold; spawned session appears in `sessions.list`.

### Phase 5 — `console.sessions.fork` — DONE
- Wraps the existing `Fork(sourceID, messageID)`. High value: "branch this conversation at message 40 and try a different approach there."

### Phase 6 — Security prerequisites landed — HALF DONE (path boundary yes, plan-mode deny no)
- Path boundary for the kernel (§8.3); `PlanMode` hard deny (§8.2). Independent of the kernel, valuable on their own.

### Phase 7 — GitHub surface: `gh` pre-auth + `console.pr.*` — HALF DONE (GH_TOKEN yes, `console.pr.*` no)
Two layers over the same stored PAT, in shipping order:

1. **GH_TOKEN injection (no kernel dependency, land first).** Every agent-spawned command inherits `GH_TOKEN=<stored PAT>` — agent `bash` (sync + background jobs), project scripts, PTY terminals — so the `gh` CLI models already reach for works on a headless box with no `gh auth login` browser flow. `gh` prefers `GH_TOKEN` over keyring/oauth state; an explicitly set `GH_TOKEN`/`GITHUB_TOKEN` always wins over the stored one. `GITHUB_TOKEN` is deliberately left unset (Actions-ephemeral convention); GHE stays out of scope, same as the git credential helper.
2. **Typed `console.pr.*` tools** over a small GitHub REST service reusing the credential store: create, read, list, update, merge. `TierWrite`, so they resolve through the same approval gating as every other write — unlike raw `gh`, which inherits the run's exec approval only. Merge is gated and never implied by create/update.

**Secret rules (both layers):** the token is loaded from the credential file at spawn/call time, never cached in a package var, never returned to any client, never logged. Exact-token occurrences are scrubbed from tool stdout/stderr before they reach the transcript, so `env` / `printenv GH_TOKEN` cannot leak it.
- **Verification**: `cd apps/server-go && go test ./tests/tools/ -run 'TestGHEnv|TestBashGH|TestBashEnvDump|TestRedactSecrets' -v` — injected when connected, absent when not, explicit value wins, raw token never in output.

---

## 10. Open Questions

- **Does `shell.read` stay native?** Recommendation: yes. It is read-tier and hot, and a native tool is cheaper per call and always present without loading a skill. `console` absorbs the *control* plane (`ports`, `project_scripts`, sessions); `shell.read` stays a thin native reader over the same implementation. Needs a decision.
- **Should Level 1 (knobbed ephemeral subagents) exist at all**, or should `subagent` jump straight to inheriting-only (Level 0) with everything else routed through `console.spawn` (Level 2)? Fewer overlapping mechanisms is appealing, but Level 1 is the cheap path for "quick side question" delegation.
- **How long should spawned sessions live?** Indefinitely (user prunes) or TTL-reaped? `ExpiredDeletedSessions(cutoffMillis)` already exists for soft-deleted sessions and could be reused.
- **Does the kernel survive a session reload?** A JS kernel holds variables that do not persist to disk. Decide whether `console` is documented as ephemeral state, or whether kernels are checkpointed.
- **Is a UI-driving surface wanted at all** (`console.ui.*`)? It is the most powerful and the least bounded. Worth deciding before building rather than after.

---

## 11. Verification Matrix

| Test Case | Method | Expected Outcome |
| :--- | :--- | :--- |
| Kernel lifecycle | `go test ./tests/tools/ -run TestConsoleKernel -v` | state persists; `console_reset` clears; session-isolated |
| Scripts parity | `go test ./tests/tools/ -run TestConsoleScripts -v` | identical to removed `project_scripts` tool |
| Ports parity | same | identical to removed `ports` tool |
| Discovery | same | invalid model id discoverable before use |
| Permission ceiling | `go test ./tests/tools/ -run TestConsoleSpawn -v` | `min(parent, requested)`; clamp reported, not silent |
| Spawn isolation | same | spawned session confined to its worktree root |
| Recursion + budget | same | depth cap 2; budget enforced; usage rolled up |
| Delete safety | same | soft by default; self-delete refused; worktree deletion confirmed |
| Path boundary | same | write outside root denied in every mode |
| Plan mode deny | `go test ./tests/agent/ -run TestPlanModeDeny -v` | `TierWrite`/`TierExec` hard-denied |
| gh pre-auth | `go test ./tests/tools/ -run 'TestGHEnv|TestBashGH|TestBashEnvDump|TestRedactSecrets' -v` | `GH_TOKEN` in child env iff connected; explicit value wins; raw token never in output |
| PR tools | same | create/update/read/list round-trip against httptest; merge gated |
| Token savings | bench | `ports` + `project_scripts` schemas gone from every request |