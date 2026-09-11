# Project Run Scripts: Server-to-Desktop Implementation Plan

## Summary

Add project-defined run scripts through a human-editable `console.toml` file. The first version supports four fields:

- `label` — required display name.
- `command` — required command to execute.
- `shortcut` — optional desktop keyboard shortcut.
- `persistent` — optional lifecycle hint for commands that are expected to remain running.

The server owns configuration loading, parsing, validation, command execution, and process state. The desktop consumes a normalized API and owns presentation, keyboard registration, and user interaction. The configuration file is TOML, while the API remains JSON.

## Proposed Configuration

The file lives at the root of the selected project:

```toml
[scripts.dev]
label = "Start Dev"
command = "bun run dev"
shortcut = "cmd-r"
persistent = true

[scripts.build]
label = "Build"
command = "bun run build"
shortcut = "cmd-shift-b"
persistent = false
```

The table key (`dev`, `build`) is the stable script identifier. It should be unique within the project and should be used by API requests rather than the human-readable label.

Only `label` and `command` are required. `shortcut` and `persistent` are optional. Missing `persistent` should be treated as `false` in the normalized API response, or represented as `null` internally before normalization. No `ports` field is included in this version; port detection and browser integration can be added separately later.

## Design Boundaries

### Server responsibilities

The server should:

1. Resolve the selected project root through the existing project/session model.
2. Read `<project-root>/console.toml`.
3. Parse TOML and validate the schema.
4. Return normalized script definitions to clients.
5. Start commands only after an explicit run request.
6. Run commands with the project root as the working directory.
7. Track process status, output, exit code, and termination.
8. Stop processes and their child process groups safely.
9. Reject invalid script identifiers, malformed configuration, unsafe paths, and invalid shortcuts.

The server should never trust a client-provided command. A run request should contain only the project/script identifier, plus any explicitly supported runtime options; the server must load the command from its own parsed project configuration.

### Desktop responsibilities

The desktop should:

1. Fetch the scripts for the active project.
2. Render the scripts in the Run UI.
3. Display validation/loading errors without crashing the workspace.
4. Start and stop scripts through the API.
5. Subscribe to status and output updates.
6. Register optional shortcuts in the appropriate Run-panel/workspace context.
7. Avoid intercepting shortcuts while text inputs, terminals, dialogs, or the browser own focus.
8. Show the configured shortcut using the platform-appropriate display form.

Keyboard shortcuts are a desktop concern. The server validates their syntax and returns them as metadata, but it does not register or execute keyboard bindings.

## Phase 1: Shared Types and Server Configuration Loader

### 1. Add a TOML parser

Add a Rust TOML parser to the desktop only if the desktop needs to read local configuration in a later phase; it should not be needed for the server implementation. For the Bun/TypeScript server, add a TOML parser dependency compatible with the existing package manager and runtime, preferably one with typed parsing support and no need to execute arbitrary code.

Keep parsing in a small server module, for example:

```text
apps/server/api/src/services/project-scripts/
  config.ts
  schema.ts
  service.ts
  types.ts
```

### 2. Define the configuration schema

Define the raw TOML shape and normalized API shape separately.

Raw configuration conceptually looks like:

```ts
type ProjectScriptsConfig = {
  scripts?: Record<string, {
    label: string;
    command: string;
    shortcut?: string;
    persistent?: boolean;
  }>;
};
```

The normalized script should include the identifier and project context:

```ts
type ProjectScript = {
  id: string;
  label: string;
  command: string;
  shortcut: string | null;
  persistent: boolean;
};
```

Validation rules:

- The root must be an object.
- `scripts`, when present, must be a table/object.
- Script identifiers must be non-empty and limited to a safe identifier format such as letters, numbers, `_`, and `-`.
- `label` must be a non-empty string with a reasonable maximum length.
- `command` must be a non-empty string with a reasonable maximum length.
- `shortcut`, when present, must be a supported keystroke string such as `cmd-r`, `cmd-shift-b`, or `ctrl-alt-l`.
- `persistent`, when present, must be a boolean.
- Unknown fields should either be rejected with a useful error or ignored consistently; rejecting them is preferable for catching typos.

Do not validate `command` by trying to execute it. Commands are intentionally shell commands and must be treated as user-authored project content.

### 3. Define missing-file behavior

A missing `console.toml` should not be an error for every project. Return an empty script list with a clear `source: "missing"` or equivalent metadata. A malformed file should return a client-visible validation error with line/column information when the parser provides it.

Do not silently fall back to `package.json` in the first implementation. That can be added later as a separate source with explicit precedence rules.

## Phase 2: Server API

### 1. Add a scripts route

Add a project-scoped route near the existing project, filesystem, terminal, and port routes:

```text
GET /api/projects/:projectId/scripts
```

Response for a valid file:

```json
{
  "success": true,
  "data": {
    "projectId": "project-id",
    "scripts": [
      {
        "id": "dev",
        "label": "Start Dev",
        "command": "bun run dev",
        "shortcut": "cmd-r",
        "persistent": true
      }
    ],
    "source": "console.toml"
  }
}
```

For a missing file, return success with an empty list. For invalid TOML or schema validation, return a structured `400` response containing an actionable error message. Use the existing project service to resolve the project path instead of accepting an arbitrary filesystem path from the client.

### 2. Add start, stop, and status operations

Use project/script identifiers rather than receiving a command from the client:

```text
POST   /api/projects/:projectId/scripts/:scriptId/run
GET    /api/projects/:projectId/scripts/runs
GET    /api/projects/:projectId/scripts/runs/:runId
POST   /api/projects/:projectId/scripts/runs/:runId/stop
```

The start response should immediately return a run record:

```json
{
  "success": true,
  "data": {
    "runId": "run-id",
    "projectId": "project-id",
    "scriptId": "dev",
    "label": "Start Dev",
    "status": "starting",
    "persistent": true,
    "startedAt": "2026-09-11T12:00:00.000Z"
  }
}
```

A run record should support at least:

- `starting`
- `running`
- `succeeded`
- `failed`
- `stopped`

The process manager should retain a small recent output buffer so the desktop can display logs that arrived before it connected. The implementation should reuse the server's existing process lifecycle patterns where practical, but a project script is a managed command execution rather than an interactive terminal session. Do not add PTY input or terminal resize behavior; the first version only needs to capture stdout/stderr and provide a stop action.

A run belongs to the server, not to a desktop window. Closing the desktop client must not stop a running script. The server keeps the process alive and keeps its current status and recent output until the process exits or the user explicitly stops it. When the desktop opens again, it lists the server's active runs and reconnects to their streams.

### 3. Stream updates

Use a simple SSE endpoint, following the existing server streaming patterns:

```text
GET /api/projects/:projectId/scripts/runs/:runId/stream
```

The endpoint streams live logs and status changes while the process is running:

```json
{ "type": "status", "status": "running" }
{ "type": "output", "stream": "stdout", "text": "ready on port 3000\n" }
{ "type": "output", "stream": "stderr", "text": "warning\n" }
{ "type": "exit", "status": "succeeded", "exitCode": 0 }
```

Keep reconnect behavior intentionally small: the desktop first fetches the current run record and recent output, then opens the SSE stream. If the desktop disconnects, the server does not stop the process. When the desktop reconnects, it fetches the current state again and resumes showing new logs. A bounded output tail is sufficient; do not add complex event replay or sequence cursors in the first version.

The stream terminates after a final exit/stopped event. For a persistent development script, it remains open until the process exits or the user clicks Stop.

### 4. Add server tests

Create dedicated tests under `apps/server/tests/` for:

- Valid TOML with all fields.
- Minimal TOML containing only `label` and `command`.
- Missing optional fields and their normalized defaults.
- Missing `console.toml`.
- Malformed TOML.
- Invalid field types.
- Invalid script identifiers.
- Invalid shortcut syntax.
- Starting a script by identifier.
- Rejecting an unknown script identifier.
- Working-directory resolution to the selected project root.
- Stop behavior and final status.
- Output and exit events.

Follow the repository rule to run only the specific relevant test file while implementing.

## Phase 3: Desktop Core Client

### 1. Add shared Rust API types

Add project-script types to the desktop core type module, following the existing service/type organization:

```rust
pub struct ProjectScript {
    pub id: String,
    pub label: String,
    pub command: String,
    pub shortcut: Option<String>,
    pub persistent: bool,
}

pub struct ScriptRun {
    pub run_id: String,
    pub project_id: String,
    pub script_id: String,
    pub label: String,
    pub status: ScriptRunStatus,
    pub persistent: bool,
    pub started_at: Option<String>,
    pub exit_code: Option<i32>,
}
```

Use serde representations consistent with the existing API types. Keep API transport models separate from UI state where needed.

### 2. Add a `ProjectScriptsService`

Add a service under the existing desktop core services module and expose it from `ConsoleClient`:

```rust
pub scripts: ProjectScriptsService,
```

The service should provide methods equivalent to:

- `list(project_id)`
- `run(project_id, script_id)`
- `list_runs(project_id)`
- `get_run(project_id, run_id)`
- `stop(project_id, run_id)`
- `stream_run(project_id, run_id, ...)`

The service must never accept a raw command from the UI for the run request.

### 3. Add stream/reconnect handling

The desktop should:

1. Fetch the current list when a project becomes active.
2. Fetch active runs when the Run UI mounts or the project changes.
3. Fetch each active run's recent output and current status.
4. Subscribe to each active run's SSE stream.
5. Re-fetch the run record and recent output after reconnect or a stream error.
6. Stop listening after a terminal status.
7. Remove or archive old runs according to a bounded UI history policy.

The desktop closing or reconnecting must not send a stop request. Only the explicit Stop action stops the server-side process. Use the existing async task and entity update patterns in the desktop app. Network work must not block GPUI rendering.

## Phase 4: Desktop Run UI

### 1. Add Run panel state

Add state scoped to the active project, not global application state:

- Loaded scripts.
- Loading/error state.
- Active runs by `run_id`.
- Output tail per run.
- Selected/expanded run.
- Shortcut registration generation/version, so stale async responses cannot replace newer project data.

When changing projects, clear or isolate the previous project's scripts and runs. Do not display a previous project's command as if it belonged to the newly selected project.

### 2. Render script cards

Each script card should show:

- Label.
- Optional shortcut badge.
- Running/stopped/completed status.
- Run button when idle.
- Stop button when running.
- Expandable output/status details.

The command can be shown as secondary information, but it should not be editable from the execution card in the first version. Configuration edits belong in `console.toml` or a later dedicated editor.

### 3. Use `persistent` as a UI hint

`persistent` does not force a process to remain alive. The actual process exit always wins.

Use it to influence presentation:

- Persistent scripts get a prominent Stop control.
- Persistent scripts remain visible in the active-runs section while running.
- The UI can warn before starting a second instance of the same persistent script.
- One-shot scripts can move to recent history after completion.

The server must still report the real process status for every script.

## Phase 5: Shortcut Support

### 1. Normalize and validate shortcut values

Use a single canonical format in `console.toml`, for example:

```text
cmd-r
cmd-shift-b
ctrl-alt-l
```

The server validates the format, but the desktop converts it to the platform display form (`⌘R`, `⌘⇧B`, etc.). Invalid shortcuts should prevent only that shortcut from registering, while the script remains runnable by clicking its Run button; the UI should show a warning.

### 2. Register shortcuts contextually in GPUI

GPUI already supports actions, `cx.bind_keys`, `KeyBinding`, key contexts, and action listeners. Use a Run-panel context rather than unconditional global bindings.

Because script identifiers are dynamic, do not create a Rust action type per project script. Use a small fixed action surface, such as:

```rust
actions!(project_scripts, [RunConfiguredScript]);
```

The handler can receive or resolve the configured target through the active Run-panel state. If the GPUI version cannot dynamically bind an arbitrary number of configured shortcuts cleanly, use a context-scoped key-down handler that normalizes the keystroke and looks it up in the current script map.

The implementation must preserve existing ownership rules:

- Text fields and the composer must keep their shortcuts.
- Terminal views must receive terminal input.
- Dialogs and menus must win while open.
- The browser's `cmd-r` reload shortcut must remain active in the browser context.
- A project shortcut should run only when the Run/workspace context is active.

Start with contextual shortcuts and a clear conflict policy. If two scripts use the same shortcut, do not run both; show a configuration warning and disable the conflicting binding until the user resolves it.

### 3. Shortcut tests

Add focused tests or integration coverage for:

- Displaying a configured shortcut.
- Running the correct script from the Run context.
- Duplicate shortcut detection.
- Browser `cmd-r` remaining reload in browser context.
- Composer/terminal input not being intercepted.
- Shortcut removal after switching projects.

## Phase 6: Errors, Security, and Operational Behavior

- Resolve project paths through the existing project registry.
- Reject path traversal in project and script identifiers.
- Never interpolate a client-provided script identifier into a shell command.
- Execute only the command loaded from the server-side `console.toml`.
- Start in the selected project root.
- Use a detached process group when needed so stopping a script also stops its child processes.
- Enforce limits for concurrent script runs, output buffer size, and output event size.
- Handle server shutdown by stopping or marking managed runs according to the existing process policy.
- Return useful errors for missing projects, missing scripts, malformed configuration, duplicate shortcuts, failed starts, and already-stopped runs.
- Avoid logging secrets from environment variables or command output beyond the existing logging policy.

## Implementation Order

1. Add and test TOML parsing plus schema normalization.
2. Add the project scripts list endpoint.
3. Add managed script run state and start/stop endpoints.
4. Add the SSE log/status stream, bounded recent output, and server tests.
5. Add desktop core API types and `ProjectScriptsService`.
6. Add desktop project-scoped Run state and initial list rendering.
7. Add start/stop controls, reconnect behavior, and live run updates.
8. Add contextual GPUI shortcut handling and conflict warnings.
9. Add desktop tests or focused integration checks.
10. Document `console.toml` in the user-facing project documentation.

## Definition of Done

- A project can define scripts in `console.toml` using only `label` and `command`.
- Optional `shortcut` and `persistent` values are parsed and normalized.
- The server, not the desktop, reads and validates the TOML file.
- The desktop can list, start, observe, and stop project scripts.
- Script output and final status are visible without blocking the UI.
- Persistent scripts are presented as long-running tasks, but actual process exit remains authoritative.
- Configured shortcuts work only in the intended Run/workspace context and do not break existing browser, composer, terminal, menu, or dialog bindings.
- Invalid configuration produces an actionable error rather than a crash or silent failure.
