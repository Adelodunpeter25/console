# Bash Tool Background Execution Spec

## Goal

Allow the **agent `bash` tool** to start long-running shell commands without keeping the tool call blocked until the process exits. The tool should return promptly with a job identifier, while the server owns the process, captures its output, reports completion, and lets a later tool call inspect or control the job.

This is not a plan to make Console's interactive terminal tabs support background processes. Terminal tabs, PTYs, terminal WebSockets, detach/reattach, and terminal-tab persistence are out of scope.

The primary use case is an agent starting a development server, watcher, build, test, or other long-running command and then continuing with another tool call:

```text
bash(command="bun dev", background=true)
→ Started background bash job `job_123`.

bash(jobId="job_123", action="output")
→ Current output and status.
```

## Current State

- `apps/server/agent/src/tools/bash.ts` executes a command through `spawnCapture`, waits for completion, captures stdout/stderr, enforces a default 30-second timeout and a 20-minute maximum, and kills the process tree on timeout or abort.
- `apps/server/api/src/utils/exec.ts` owns the shared `Bun.spawn` capture and process-tree termination behavior. It currently returns only after the subprocess exits or is killed.
- `apps/server/agent/src/service/tool-executor.ts` validates tool input, resolves approval, calls `tool.execute`, and returns one `ToolResult` for the tool call. A background result must be returned immediately while later job events remain server-side until explicitly requested.
- `apps/server/agent/src/types/tool.ts` defines the tool schema and result contracts. Tool input is currently specific to the `bash` tool; there is no background-job registry or job-management API.
- `apps/server/tests/tools.test.ts` covers synchronous bash execution, output, timeout limits, and schema behavior.
- The interactive terminal implementation under `apps/server/api/src/terminal` is a separate PTY/WS feature and should not be changed for this work.

## Desired UX and Tool Contract

### Starting a job

Extend the `bash` input schema with an optional `background` flag:

- `background: false` or omitted preserves the current synchronous behavior.
- `background: true` starts the command and returns without waiting for completion.
- The existing `command`, `cwd`, `env`, and timeout validation remains in force.
- Background execution must use the same approval tier and permission flow as synchronous bash execution.

A successful start returns structured text or a structured payload containing:

- `jobId` — a unique opaque identifier.
- `command` and resolved working directory.
- `status: "running"`.
- Process start time.
- The output retrieval instructions.

The initial response must not claim that the command succeeded; it only confirms that the process started.

### Inspecting a job

Choose one of these approaches during implementation, preferring the smallest API that works with the existing tool model:

1. Add optional `jobId` and `action` fields to `bash` so the same tool supports `status`, `output`, `wait`, and `kill`.
2. Add a separate `bashJob` tool for job management, keeping the existing `bash` schema focused on starting commands.

The management contract should support:

- `status`: Return `running`, `exited`, `failed`, `killed`, or `expired`, plus exit code and timestamps when available.
- `output`: Return new output since a cursor/sequence value, or a bounded recent-output window when no cursor is supplied.
- `wait`: Wait for completion up to a caller-provided limit, then return current status and output without killing the job.
- `kill`: Terminate the process tree and mark the job as killed.

The model must be able to distinguish a failed command from a failed job lookup and from a job that is still running.

### Completion

A completed job should retain:

- Exit code or signal.
- `startedAt`, `finishedAt`, and timeout/abort state.
- Bounded stdout and stderr output.
- The final status.

Completion should be discoverable through a later `status`, `output`, or `wait` call. Do not depend on a terminal tab, terminal WebSocket, or a separate desktop notification to deliver the result.

## Design

### 1. Server-side background job manager

Create a process manager in the server/agent execution layer, close to the existing `spawnCapture` integration. It should:

- Generate an unguessable `jobId`.
- Store the subprocess handle, command metadata, output buffers, status, timestamps, and owner/session information.
- Drain stdout and stderr continuously so a child cannot block on a full pipe.
- Preserve output in bounded per-stream buffers, preferably a byte-limited ring buffer with sequence numbers.
- Resolve completion once and retain the terminal result for a limited time.
- Reuse the existing process-tree termination logic for explicit kill, timeout, cancellation, and server shutdown.
- Keep all subprocess handles and callbacks on the Bun/server event loop; no desktop PTY is involved.

The registry should be explicitly scoped. At minimum, jobs need an owner such as agent session ID; job IDs must not allow one session to inspect or kill another session's process.

### 2. Background timeout and cancellation

Backgrounding changes when the tool returns, not whether resource limits apply:

- Preserve a maximum runtime. The existing 20-minute maximum may be a starting default, but long-lived development servers need a separately documented policy rather than an unbounded process.
- `timeoutMs` should define the maximum lifetime for a background job, not the time the initial tool call waits.
- An agent abort should cancel the synchronous tool call as it does today. Decide explicitly whether it also kills jobs started by that call; the safer default is to kill them unless the user has intentionally created a durable background job.
- Server shutdown must terminate all background jobs and report that they were interrupted when their status is next requested.
- Add an idle retention period for completed jobs so the registry cannot grow without bound.

### 3. Output and backpressure

The synchronous tool currently caps output at `MAX_OUTPUT_BYTES` and drains excess data. Background jobs need separate lifecycle and retrieval limits:

- Use a bounded ring buffer for recent stdout and stderr.
- Include `truncated: true` and byte/sequence metadata when older output has been discarded.
- Return output incrementally with a cursor so repeated polling does not resend the entire buffer.
- Keep stdout and stderr distinguishable.
- Never allow unbounded output from watchers or development servers to consume server memory.

### 4. Tool integration and approvals

- Keep `bash` as an `exec`-tier tool and run the existing approval resolution before spawning either synchronous or background commands.
- Do not bypass approval because the initial background response is fast.
- Do not expose raw subprocess handles, PIDs, or arbitrary registry internals to the model.
- Validate `jobId`, actions, cursors, and wait limits with Zod.
- Use the existing tool-result normalization and error conventions.
- Make background job ownership part of the agent/session lifecycle so stale jobs can be cleaned up safely.

### 5. Shell behavior

Background mode should execute the same shell command flavor as synchronous mode (`/bin/sh -c` on Unix and `cmd.exe /c` on Windows), with the same resolved `cwd` and environment behavior.

Do not rely on shell syntax such as appending `&`, `nohup`, or redirecting to files. Those approaches detach the child from the tool manager, make status and cleanup unreliable, and can cause output loss. The manager—not the model-authored shell string—owns the background lifecycle.

Commands that daemonize themselves, fork away from the process tree, or intentionally detach may not be trackable. Document this limitation and prefer managed foreground processes in background mode.

## Data Model

A conceptual job record:

```ts
type BashJobStatus = "running" | "exited" | "failed" | "killed" | "expired";

interface BashJob {
  jobId: string;
  ownerSessionId: string;
  command: string;
  cwd: string;
  status: BashJobStatus;
  exitCode?: number;
  signal?: string;
  startedAt: string;
  finishedAt?: string;
  timedOut: boolean;
  aborted: boolean;
  stdout: BoundedOutput;
  stderr: BoundedOutput;
}
```

The exact type can live in the agent/server layer rather than the shared terminal types package. Do not add background-job concepts to the terminal WS protocol or `WorkspaceTabConfig`.

## Out of Scope

- Background execution for interactive terminal tabs.
- PTY detach/reattach or terminal WebSocket changes.
- Keeping terminal tabs alive when their desktop pane is closed.
- Desktop terminal badges, terminal pickers, or terminal notifications.
- Subagent run backgrounding and run-level detach.
- SQLite persistence or surviving a server restart.
- Arbitrary process adoption after shell-level daemonization.
- A general-purpose process supervisor.

A future terminal-specific proposal may address PTY lifecycle independently, but it must not be combined with this bash-tool implementation.

## Alternatives Considered

- **Append `&` to the command** — rejected because the tool loses reliable process ownership, exit status, output capture, and cleanup.
- **Reuse the interactive terminal PTY manager** — rejected because bash tool calls are non-interactive subprocesses and coupling them to terminal tabs would create unwanted desktop/WS state.
- **Block the agent loop until the command exits** — rejected because development servers and watchers prevent the agent from continuing.
- **Persist jobs in SQLite** — deferred; jobs are tied to the in-memory server process and should be terminated on restart.
- **Create a separate process-service API first** — defer unless the existing tool-layer registry cannot provide the required ownership and cleanup semantics.

## Implementation Steps

1. `apps/server/agent/src/tools/bash.ts` — add `background` input and the management action contract, while preserving synchronous behavior when it is omitted.
2. `apps/server/agent/src/service/` or a focused new module — implement the in-memory `BashJobManager`, job ownership, status transitions, bounded output, polling cursors, retention, and cleanup.
3. `apps/server/api/src/utils/exec.ts` — extract or extend reusable process spawning/output-draining helpers without changing interactive terminal behavior.
4. `apps/server/agent/src/service/tool-executor.ts` — ensure background start and management actions use the existing validation, approval, cancellation, and result normalization paths.
5. `apps/server/agent/src/types/tool.ts` or a server-local type module — add only the shared types needed by the bash tool; do not add them to terminal message types.
6. Add focused tests for start, status, incremental output, wait, kill, timeout, abort, ownership, retention, output caps, and process-tree cleanup.
7. Update the agent system prompt/tool description with when to use background mode, how to poll, and the warning that the initial response is not an exit result.

## Verification

- Synchronous `bash` behavior remains unchanged when `background` is omitted.
- `bash(background=true, command="sleep 2")` returns a running job ID promptly rather than waiting two seconds.
- A later status call observes the transition to `exited` and the correct exit code.
- A background command's stdout and stderr are available through output polling and are not lost when no poll occurs while it runs.
- Incremental polling does not duplicate output and reports truncation after the buffer limit is exceeded.
- `wait` returns final output for a short command and a still-running status when its wait limit expires.
- `kill` terminates the process tree and returns a killed status.
- Background timeout kills the process tree and reports that it timed out.
- Agent cancellation follows the documented policy and does not leave an orphaned process.
- A job ID from another agent session cannot be read or killed.
- Completed-job retention and maximum concurrent background-job limits are enforced.
- A command that writes continuously cannot exhaust server memory or block because output is not being read.
- Server shutdown cleans up running jobs.
- Run the focused server tests for the bash tool and job manager; do not use or modify `terminal.test.ts` for this feature.

## Risks and Mitigations

- **Orphaned processes**: Track process groups, kill the process tree, clean up on shutdown, and apply maximum runtimes.
- **Unbounded output**: Drain continuously and enforce bounded ring buffers with explicit truncation metadata.
- **Stale job records**: Expire completed jobs and enforce concurrent-job limits.
- **Cross-session access**: Associate every job with an owner session and authorize every management action.
- **Model confusion about completion**: Return explicit `running` status and document that start is not success.
- **Shell self-detachment**: Document that commands must remain in the managed process tree for reliable tracking.
- **Platform differences**: Keep process-tree termination and shell invocation behind the existing server execution helpers and test Unix and Windows paths where available.
