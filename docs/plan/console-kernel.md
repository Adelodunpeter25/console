# The `console` kernel

What exists, how it behaves, and what was decided against. The kernel gives an agent a control plane over the app: run project scripts, manage ports, work with sessions. It lives in `internal/services/console/`.

It replaced the separate `ports` and `project_scripts` agent tools with one JavaScript tool (`console`, plus `console_reset`), the same move `computer` made for computer use: one small schema on every turn, and the method reference loaded on demand through a skill (`internal/services/console/SKILL.md`).

## The API

Every method takes **one object**, never positional values.

```js
// Discovery
console.models()                          console.approvalModes()
console.projects.list()                   console.projects.add({ directory, name? })

// Sessions
console.sessions.list({ project_id?, cwd? })
console.sessions.read({ session_id, limit?, before? })
console.sessions.search({ query, session_id? | project_id? | cwd? | all_projects?, roles?, limit?, per_session? })
console.sessions.create({ prompt?, project_id?, cwd?, model?, provider?, thinking_level?, approval_mode?, worktree? })
console.sessions.prompt({ session_id, prompt, model?, provider?, thinking_level?, approval_mode? })
console.sessions.queue({ session_id })    console.sessions.clear_queue({ session_id })
console.sessions.wait({ session_id | session_ids, mode?, timeout_ms? })
console.sessions.abort({ session_id })
console.sessions.set_model({ session_id, model, provider? })
console.sessions.set_approval({ session_id, mode })
console.sessions.fork({ session_id, message_id })
console.sessions.delete({ session_id })

// Scripts and ports
console.scripts.list()                    console.scripts.run({ id, wait_ms? })
console.scripts.status({ run_id? })       console.scripts.output({ run_id, stream?, cursor?, limit? })
console.scripts.stop({ run_id })
console.ports.list()   console.ports.forward({ port })   console.ports.unforward({ port })

// MCP servers
console.mcp.list()                        console.mcp.tools({ id })
console.mcp.add({ id, transport, url | command, args?, env?, label?, auth?, enabled? })
console.mcp.update({ id, label?, url?, command?, args?, env?, auth?, tier_overrides?, enabled? })
console.mcp.remove({ id })
console.mcp.connect({ id })               console.mcp.disconnect({ id })
console.mcp.reset_auth({ id })            console.mcp.call({ id, tool, args? })
```

Not built: `console.pr.*` (see [`pull-requests.md`](pull-requests.md)), and any UI-driving surface.

## How it behaves

- **Plain results.** A call returns its data directly (`console.sessions.list()` is the array), and a failure throws with the reason. Only `console` does this; computer use keeps a wrapper because it returns images.
- **Strict arguments.** Every method lists the keys it accepts (`methodArgs` in `runtime.go`), and a test checks that list against the installed methods. An unknown key or a wrong type fails with the accepted names, so `{projectId: ...}` is caught instead of ignored.
- **Helpful errors.** An unknown method or namespace lists the valid ones. The timeout message says what to check. Output past 100,000 characters is cut with a note saying how much, and `sessions.read` takes `limit`/`before` to ask for less.
- **`sessions.create`** with a prompt starts it (`started: true`, `status: working`).
- **`sessions.prompt`** starts an idle session or queues the prompt for a busy one (`queued: true`). One prompt can wait per session and it never replaces one, including the user's own. `sessions.abort` also clears the queue, so an aborted session does not restart itself.
- **`sessions.wait`** collects results of started work. One id or a list (max 20), `mode: "all"` (default) or `"any"`, at most 60 seconds per call so it fits inside the kernel's 5-minute timeout; on timeout it returns `done: false` and the caller waits again. Each session reports `done`, `status` and `last_reply` (cut at 20,000 characters). It refuses to wait on the calling session.
- **`console.mcp.*`** manages MCP servers through the same `mcp.Manager` the desktop settings page uses, so what the model changes is what the UI shows. Discovery (`list`, `tools`), connection control (`connect`, `disconnect`, `reset_auth`) and `call` (one remote tool, the same path the `loadTools` adapter tools take) are available to any run that can call `console` at all. `add`, `update` and `remove` change user-level config that affects every session and project — and a stdio server is a command that runs on the machine — so they demand the run's ceiling to be `full-access` and fail closed on an unknown ceiling. `add` never takes a credential: the model passes an auth type, tokens live in Settings, and an OAuth connect returns `needs_auth` with the `auth_url` for the user instead of waiting out the browser sign-in.
- **`sessions.search`** finds messages containing every word of a query, over what people said (never tool output, thinking, images or injected messages). By default it searches the current project (or directory) and leaves out the running session. It calls `session.Service.Search`, the same function an in-chat Cmd+F will use. See [`message-storage.md`](message-storage.md).
- **Scripts:** `scripts.output` pages a run's output with absolute-offset cursors that stay valid when the 256 KB buffer drops its head.

## The permission ceiling

A run may never hand out more authority than it holds: `effective = min(run's mode, requested mode)`, with the order `plan-mode < always-ask < accept-edits < full-access`. It is enforced for `sessions.create`, `set_approval`, `prompt` (the turn's mode) and `fork`. The result reports the clamp in `clamped` so a model does not retry the same call hoping for a different answer. With no run behind the kernel the ceiling fails closed to `plan-mode`. A run cannot abort itself, wait on itself, prompt itself, delete itself, or change its own approval mode.

This invariant comes from subagents, which inherit the parent's tools and so must inherit its authority too (`internal/agent/loop/subagent.go`).

## Plan mode

Plan mode is read-only and enforced by the server, not requested of the model.

- `permissions.Resolve` denies the write and exec tiers and never prompts. Reads, `browser` and `memory` (read tier) still run.
- `writePlan` is the one write allowed. It saves `docs/plan/<name>.md`, creating the folder if needed. The model gives only a name; the tool builds the filename, **never overwrites** (a taken name gets `-01`, `-02`, ...), refuses symlinks, and has no size cap. It is offered only in plan mode.
- `bash` runs under a read-only OS sandbox in plan mode only (`internal/agent/tools/sandbox*.go`): the kernel refuses every file write outside the system temp folder, whatever the command string says. The network stays open, reads are unrestricted (secrets included, by decision), and `git push` works. macOS uses `sandbox-exec`; Linux uses `bwrap`. A probe really tries a write once; if the sandbox does not work, or there is no sandbox tool (or the OS is Windows), plan-mode bash runs **unsandboxed**, so on such a machine plan mode is not read-only for shell commands. `bashJob` stays denied.
- Subagents inherit the mode.
- Accepted gaps: `browser` can still submit forms, and MCP tools are tiered by naming convention.

**Verified:** macOS, with the real sandbox and mutation checks. **Not verified:** the Linux `bwrap` path (it compiles and type-checks). On a Linux machine run `go test ./tests/tools/ -run "Sandbox|PlanBash"`; it skips if `bwrap` is missing.

File writes are also confined to the session's working directory (`permissions/guard.go`): `write_file`, `editFile` and `batchWrite` may only target paths inside it, symlinks resolved first. `bash` is not path-confined outside plan mode, because a command string is opaque (`cd ../other && ...` walks past any check). That is accepted.

## Capabilities (`/commands`)

`/console`, `/visualize` and `/computer-use` are declared once in a table (`internal/run/capabilities.go`); the run loop, the slash-command popup and `readSkill` all read it. Which are on is stored in the session row (`sessions.capabilities`), so it survives a restart. An agent cannot type a slash command, so `readSkill` knows them: `readSkill()` lists them with their state, and `readSkill("visualize")` or `("console")` returns the reference **and switches the capability on**. `computer-use` needs the user's consent and OS permissions, so `readSkill("computer-use")` tells the agent to ask the user to type `/computer-use`. Adding a capability is one entry in the table.

## GitHub

`GH_TOKEN` is injected into spawned commands (`providers/github/ghenv.go`) and scrubbed from output, so `gh` works without a browser login. The typed `console.pr.*` tools are designed in [`pull-requests.md`](pull-requests.md) and not built.

## Decided against

- **Spawn limits** (depth cap, concurrency cap, per-spawn token budget, usage rollup). The risk is small: models follow an instruction not to fan out, and a spawned session cannot hold more authority than its creator. They would also need a parent link on sessions, run-service accounting and a cancel-at-budget path. If a real runaway ever happens, the order that makes sense is parent link and depth cap first (the rest depend on it), then concurrency, then rollup, and the budget last.
- **A separate `console.spawn`.** Creating a task is `sessions.create`, which already carries every option.
- **Per-agent `cwd`.** The file tools are process-wide singletons resolving paths directly, so making `cwd` real is a larger refactor. Isolation comes from worktrees (`worktree: { branch, base_branch }`), which already exist.

## Ideas, not built

- **Wake the parent when a child finishes.** `sessions.wait` makes the caller loop; waking the parent with the child's result would need no call at all, but needs a new kind of wake event.
- **How long spawned sessions live.** Currently until the user deletes them. `ExpiredDeletedSessions` could reap soft-deleted ones on a TTL.
- **Whether knobbed ephemeral subagents** (model/provider overridable) should exist, or `subagent` stays inherit-everything with other delegation going through `sessions.create`.
- **A UI-driving surface** (`console.ui.*`) is the most powerful and least bounded option. Decide before building, not after.

## Tests

Kernel tests live beside the code in `internal/services/console/`. Capability registry: `internal/run/capabilities_test.go`. Script cursors: `tests/api/scripts_output_cursor_test.go`. MCP methods: `internal/services/console/mcp_test.go`. Path guard and plan mode: `tests/permissions/`. Sandbox: `tests/tools/plan_sandbox_test.go`. GitHub token: `tests/tools/bash_gh_env_test.go`.
