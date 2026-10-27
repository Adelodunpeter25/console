# Console control

You have access to the console API through the `console` tool. This tool runs
JavaScript with a preinstalled `console` object that gives you control over the
desktop app: manage ports, run project scripts, spawn sessions, and inspect and
steer them. Variables persist between calls until `console_reset` clears them.

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

Port operations are scoped to the current session's project. The `/console`
message activates this API for the session; the command text is not rewritten or
saved separately.

### Discovery (coming in Phase 3)

`console.models()` — list available models, providers, and supported thinking levels

`console.approvalModes()` — list valid approval modes and their ordering

`console.sessions.list({status?, project_id?})` — list sessions, filterable by status and project

### Session control (coming in Phase 3)

`console.sessions.read(session_id)` — read a session's transcript, status, and result

`console.sessions.fork(session_id, message_id)` — fork a session at a specific message

`console.sessions.delete(session_id)` — soft-delete a session (never self; gated when it owns a worktree)

`console.sessions.set_model(session_id, model, provider)` — change a session's model

`console.sessions.set_approval(session_id, mode)` — change a session's approval mode (clamped to the owning run's ceiling)

`console.sessions.abort(session_id)` — abort a running session

### Scripts

`console.scripts.list()` — list available project scripts

`console.scripts.run(id, {wait_ms?})` — run a project script. `wait_ms` is capped at 10 seconds.

`console.scripts.status(run_id?)` — inspect one run or list runs for this checkout

`console.scripts.stop(run_id)` — stop a running script

### Spawn (coming in Phase 4)

`console.spawn({prompt, title, model?, provider?, thinking_level?, approval_mode?, worktree?})` — spawn a new session as a task. Returns `{session_id}` and reports the approval mode clamp if the requested mode exceeds the parent's ceiling.
