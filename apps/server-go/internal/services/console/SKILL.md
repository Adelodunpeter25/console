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

`console.sessions.create({prompt?, project_id?, cwd?, model?, provider?, thinking_level?, approval_mode?, worktree?})` — create a persistent session and, when `prompt` is provided, start it; `worktree` may include `branch` and `base_branch`. `thinking_level` needs an explicit `model` and must be one of that model's levels (see `console.models()`). `approval_mode` is clamped to this run's own mode and defaults to it; the result reports `approval_mode` and, when lowered, a `clamped` note — do not retry to get more

`console.sessions.delete(session_id)` — soft-delete a session; running sessions must be stopped first

`console.sessions.abort({session_id})` — stop a session's active run. Fails if the session has no active run, and you cannot abort the session you are running in.

`console.sessions.set_model({session_id, model, provider?})` — change a session's model; `provider` defaults to the session's current one. Unknown ids are refused, so check `console.models()` first.

`console.sessions.set_approval({session_id, mode})` — change a session's approval mode. A session can be moved down the scale, never above this run's own mode; a higher request is clamped and reported in `clamped`. You cannot change the mode of the session you are running in.

`console.sessions.fork({session_id, message_id})` — copy a session's history through `message_id` (inclusive) into a new idle session. Message ids are on the messages from `console.sessions.read`. The fork's approval mode is clamped to this run's mode.

### Scripts

`console.scripts.list()` — list available project scripts

`console.scripts.run(id, {wait_ms?})` — run a project script. `wait_ms` is capped at 10 seconds.

`console.scripts.status(run_id?)` — inspect one run or list runs for this checkout

`console.scripts.output({run_id, stream?, cursor?, limit?})` — read a run's output in pages. `stream` is `stdout` (default) or `stderr`. Returns `{text, from, next, truncated}`; pass `next` back as `cursor` to read only what is new. Cursors are absolute byte offsets: output past the 256KB buffer is dropped from the head, and a cursor that fell behind resumes at the oldest kept byte with `truncated: true`.

`console.scripts.stop(run_id)` — stop a running script


