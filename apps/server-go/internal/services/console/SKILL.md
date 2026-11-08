# Console

You have access to the `console` API through the `console` tool: a persistent
JavaScript kernel for operating the app — ports, project scripts, sessions,
projects and MCP servers. Variables persist between calls until `console_reset`
clears them.

## Calling

```js
var sessions = console.sessions.list();
print(sessions);
```

Methods already wait for their result. **Every method takes a single object,
never positional values**: `console.sessions.read({session_id: id})`, not
`read(id)`. A method with no arguments takes none. Unknown keys and wrong
types are refused with the names the method accepts, so a typo such as
`{projectId: ...}` fails instead of being ignored. Ports and counts must be
whole numbers (`{port: 3000}`). A call returns its data directly (an object or
array), so `console.sessions.list()` is the array of sessions, not a wrapper
around it. A failed call throws an error with the reason; read the message
before deciding what to do next. Results over about 100,000 characters are cut
off with a note saying how much was lost — ask for less (see `limit` on
`sessions.read`).

`print()` narrates and shows intermediate values. The final value is returned
to you as well.

## Discovering methods

The API is large; do not guess method names or arguments. Look them up:

- `console.help()` — list every method, grouped results
- `console.help({query: "wait for session"})` — search names and descriptions
- `console.help({namespace: "sessions"})` — list one namespace's methods
- `console.describe({name: "sessions.create"})` — full argument docs and an
  example call for one method

Namespaces: `ports`, `scripts`, `sessions`, `projects`, `automation`, `mcp`, plus
`models`, `approvalModes`, `help` and `describe` at the top level.

## Rules that always apply

- A run can never grant a session more authority than it holds: approval modes
  are clamped to the run's own mode and the result says so in `clamped`.
- You cannot abort, prompt, delete or reconfigure the session you are running
  in — finish your turn instead.
- `sessions.create({prompt})` starts work; collect the result with
  `sessions.wait` (use `background: true` for anything long) rather than
  polling `sessions.read`.
- MCP config writes (`mcp.add`, `mcp.update`, `mcp.remove`) require
  full-access; they affect every session and project.
