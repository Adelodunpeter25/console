# Console actions

You have access to the `console` API through the `console` tool. Use it to
perform project actions: inspect or forward ports, run project scripts, discover
models and projects, and create or inspect sessions. Variables persist between
calls until `console_reset` clears them.

## Calling

```js
var ports = console.ports.list();
print(ports.structuredContent);
```

Calls are synchronous: no `async`, no `await`, no `.then` — every method
already waits for its result, and async completions are refused rather than
left hanging. Arguments are a single object; a method with no arguments takes
none. Every call returns `{content, structuredContent, isError}`. If
`isError` is true, read the message before deciding what to do next.

`print()` narrates and shows intermediate values. The final value is returned
to you as well.

## Methods

### Ports

`console.ports.list()` — list all forwarded ports

`console.ports.forward({port})` — forward a local port. Returns the forwarded port object.

`console.ports.unforward({port})` — remove a port forward. Returns `{success: true}`.

Port operations are scoped to the current session's project. Use these methods
when a project action requires inspecting or changing a local port forward.

### Discovery

`console.models()` — list available providers and models, including thinking levels

`console.approvalModes()` — list valid approval modes with descriptions and the restriction order (`plan-mode` < `always-ask` < `accept-edits` < `full-access`). A session may be moved down this scale, never up past the invoking run.

`console.projects.list()` — list registered projects

`console.projects.add({directory, name?})` — register a project directory

### Session control

`console.sessions.list({project_id?, cwd?})` — list persisted, non-deleted sessions, optionally filtered

`console.sessions.read(session_id)` — read a session header and transcript

`console.sessions.create({prompt?, project_id?, cwd?, model?, provider?, approval_mode?, worktree?})` — create a persistent session and, when `prompt` is provided, start it; `worktree` may include `branch` and `base_branch`

`console.sessions.delete(session_id)` — soft-delete a session; running sessions must be stopped first

### Scripts

`console.scripts.list()` — list available project scripts

`console.scripts.run(id, {wait_ms?})` — run a project script. `wait_ms` is capped at 10 seconds.

`console.scripts.status(run_id?)` — inspect one run or list runs for this checkout

`console.scripts.stop(run_id)` — stop a running script


