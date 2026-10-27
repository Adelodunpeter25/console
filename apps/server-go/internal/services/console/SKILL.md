# Console control

You have access to the console API through the `console` tool. This tool runs
JavaScript with a preinstalled `console` object that gives you control over the
desktop app: manage ports, run project scripts, list projects and models, and
create, inspect, or delete sessions. Variables persist between calls until
`console_reset` clears them.

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

### Discovery

`console.models()` — list available providers and models, including thinking levels

`console.projects.list()` — list registered projects

`console.projects.add({directory, name?})` — register a project directory

### Session control

`console.sessions.list({project_id?, cwd?})` — list active sessions, optionally filtered

`console.sessions.read(session_id)` — read a session header and transcript

`console.sessions.create({prompt?, project_id?, cwd?, model?, provider?, approval_mode?, worktree?})` — create a persistent session and, when `prompt` is provided, start it; `worktree` may include `branch` and `base_branch`

`console.sessions.delete(session_id)` — soft-delete a session; active sessions must be stopped first

### Scripts

`console.scripts.list()` — list available project scripts

`console.scripts.run(id, {wait_ms?})` — run a project script. `wait_ms` is capped at 10 seconds.

`console.scripts.status(run_id?)` — inspect one run or list runs for this checkout

`console.scripts.stop(run_id)` — stop a running script

There is intentionally no `console.spawn()`: persistent delegated work uses
`console.sessions.create()`.
