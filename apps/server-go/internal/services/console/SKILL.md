# Console actions

You have access to the `console` API through the `console` tool. Use it to
perform project actions: inspect or forward ports, run project scripts, discover
models and projects, and create or inspect sessions. Variables persist between
calls until `console_reset` clears them.

## Calling

```js
var ports = console.ports.list();
print(ports);
```

Calls are synchronous: no `async`, no `await`, no `.then` — every method
already waits for its result, and async completions are refused rather than
left hanging. **Every method takes a single object, never positional values**:
`console.sessions.read({session_id: id})`, not `read(id)`. A method with no
arguments takes none. Unknown keys and wrong types are refused with the names
the method accepts, so a typo such as `{projectId: ...}` fails instead of being
ignored. Ports and counts must be whole numbers (`{port: 3000}`). A call returns its data directly (an object or array), so
`console.sessions.list()` is the array of sessions, not a wrapper around it. A
failed call throws an error with the reason; read the message before deciding
what to do next. Results over about 100,000 characters are cut off with a note
saying how much was lost, so ask for less (see `limit` on `sessions.read`).

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

`console.approvalModes()` — list valid approval modes with descriptions and the restriction order (`plan-mode` < `full-access`). A session may be moved down this scale, never up past the invoking run.

`console.projects.list()` — list registered projects

`console.projects.add({directory, name?})` — register a project directory

### Session control

`console.sessions.list({project_id?, cwd?})` — list persisted, non-deleted sessions, optionally filtered

`console.sessions.read({session_id, limit?, before?, include_deleted?})` — read a session header and transcript. Without `limit` you get every message, which can be very large; pass `limit` for just the most recent N. The result has `hasMore` and `nextCursor`: pass `nextCursor` back as `before` to read further back. Pass `include_deleted: true` to read a soft-deleted (trashed) session, such as one `sessions.search({include_deleted: true})` found; without it a trashed session reads as not found (the error says so).

`console.sessions.create({prompt?, project_id?, cwd?, model?, provider?, thinking_level?, approval_mode?, worktree?})` — create a persistent session and, when `prompt` is provided, start it (the result has `started: true` and the session's `status` is `working`); `worktree` may include `branch` and `base_branch`. Title is automatically generated on the first prompt run. `model`, `provider`, and `thinking_level` default to this session's own settings. `approval_mode` is clamped to this run's own mode and defaults to it; the result reports `approval_mode` and, when lowered, a `clamped` note — do not retry to get more

`console.sessions.delete({session_id})` — soft-delete a session; running sessions must be stopped first. You cannot delete the session you are running in.

`console.sessions.abort({session_id})` — stop a session's active run. Fails if the session has no active run, and you cannot abort the session you are running in.

`console.sessions.set_model({session_id, model, provider?})` — change a session's model; `provider` defaults to the session's current one. Unknown ids are refused, so check `console.models()` first.

`console.sessions.set_approval({session_id, mode})` — change a session's approval mode. A session can be moved down the scale, never above this run's own mode; a higher request is clamped and reported in `clamped`. You cannot change the mode of the session you are running in.

`console.sessions.queue({session_id})` — show the next prompt waiting to run on a session (`{queued: false}` when none), so you can confirm a queued prompt before it runs.

`console.sessions.clear_queue({session_id})` — discard every waiting prompt (`{cleared: true|false}`). `console.sessions.abort` also clears it, so an aborted session does not start working again.

`console.sessions.prompt({session_id, prompt, model?, provider?, thinking_level?, approval_mode?})` — send a follow-up prompt to an existing session. An idle session starts working on it now; a busy one queues it to run when its current run finishes, and the result says `queued: true`. Queued prompts run in the order they were queued, so yours goes behind any the user already staged. The turn's approval mode is capped at this run's own mode even if the target session stores a higher one (reported as `clamped`); the session's stored mode is not changed. You cannot prompt the session you are running in. Sending the prompt does not wait for the reply: read the result later with `console.sessions.read`.

`console.sessions.wait({session_id | session_ids, mode?, timeout_ms?, background?})` — block until the named sessions finish their current run, so you can collect the result of work you started with `sessions.create` or `sessions.prompt` instead of polling `sessions.read`. Pass one `session_id` or a list in `session_ids` (up to 20). `mode` is `"all"` (default, wait for every session) or `"any"` (return as soon as one finishes). `timeout_ms` defaults to 30 seconds and is capped at 60: on timeout you get `done: false`, `timed_out: true` and the sessions still running. Each session in the result has `done`, `status` and, once finished, `last_reply` (its final assistant message, cut at 20,000 characters; use `sessions.read` for the whole transcript). You cannot wait on the session you are running in.

**Use `background: true` for anything that may take more than a minute; never call the blocking wait in a loop.** A background wait returns at once with `{background: true, work_id, max_wait_ms, sessions}` and keeps waiting as background work, like a background `bash` job: when the sessions finish you are notified automatically with each session's status and last reply, even if you have ended your turn. So end your turn or carry on with other work; do not wait again or poll. In this mode `timeout_ms` defaults to 30 minutes and is capped there. If it runs out you are still notified, told which sessions are still running (they are not stopped), and can start another background wait. The delivered replies are cut short to fit: for a full last reply call `sessions.wait` on the same sessions (it returns at once now they are finished) or use `sessions.read`. If the sessions are already finished when you call it, you get the normal blocking result straight away and no background work is started.

`console.sessions.search({query, session_id? | project_id? | cwd? | all_projects?, roles?, limit?, per_session?, include_deleted?})` — find messages that contain every word of `query` (case-insensitive, up to 8 words). It searches what people said: the user's messages, the assistant's replies, and one line per tool call (`read_file src/auth.go`). It never searches tool output, thinking, images or server-injected messages. By default it searches **this session's project** (or its directory when there is no project) and **leaves out the session you are running in**, since that is already in your context; pass `project_id`, `cwd`, `session_id` or `all_projects: true` to widen or narrow it (choose one). Pass `include_deleted: true` to also search soft-deleted (trashed) sessions; their hits carry `session_deleted: true`, and you read one with `console.sessions.read({session_id, include_deleted: true})`. `roles` is `["user"]`, `["assistant"]` or both (default). Results are newest first, at most 20 in all and 3 per session by default (`limit` up to 100, `per_session`). Each hit has `session_id`, `session_title`, `message_id`, `role`, `created_at`, a one-line `snippet`, and `matches` (character offsets of every occurrence in the message's text). Check `truncated`: if true the search stopped early and said why in `note`, so no hits is not the same as nothing exists. Follow a hit with `console.sessions.read({session_id})` to read the conversation around it.

`console.sessions.fork({session_id, message_id})` — copy a session's history through `message_id` (inclusive) into a new idle session. Message ids are on the messages from `console.sessions.read`. The fork's approval mode is clamped to this run's mode.

### Scripts

`console.scripts.list()` — list available project scripts

`console.scripts.run({id, wait_ms?})` — run a project script. `wait_ms` is capped at 10 seconds.

`console.scripts.status({run_id?})` — inspect one run, or list runs for this checkout when no `run_id` is given

`console.scripts.output({run_id, stream?, cursor?, limit?})` — read a run's output in pages. `stream` is `stdout` (default) or `stderr`. Returns `{text, from, next, truncated}`; pass `next` back as `cursor` to read only what is new. Cursors are absolute byte offsets: output past the 256KB buffer is dropped from the head, and a cursor that fell behind resumes at the oldest kept byte with `truncated: true`.

`console.scripts.stop({run_id})` — stop a running script

### MCP servers

Manage the app's MCP server integrations. Configuration methods change
user-level state that affects every session and project, so `mcp.add`,
`mcp.update` and `mcp.remove` only run when this run is at `full-access`; the
read and connection methods work everywhere.

`console.mcp.list()` — list every configured server with its live state: `status` (`disconnected`, `connecting`, `needs_auth`, `connected`, `error`), `toolCount`, and per-server `tools` when connected.

`console.mcp.tools({id})` — one server's tool list and status.

`console.mcp.add({id, transport, url | command, args?, env?, label?, auth?, enabled?})` — register a server. `transport` is `"http"` (needs `url`) or `"stdio"` (needs `command`, plus optional `args` array and `env` object). `auth` is `"none"` (default), `"static"` or `"oauth2"`; the credential itself is never passed here — tokens are managed in Settings → MCP and OAuth sign-in happens in the user's browser. The id must match `[A-Za-z0-9_-]+`.

`console.mcp.update({id, label?, url?, command?, args?, env?, auth?, tier_overrides?, enabled?})` — change an existing server; absent keys keep their stored value. Setting `enabled: false` also disconnects it. `tier_overrides` maps a tool name to `read`, `write` or `exec`.

`console.mcp.remove({id})` — disconnect and delete a server's config.

`console.mcp.connect({id})` — connect a server and wait briefly. A normal server returns `{connected: true, tool_count}` or fails with the reason. An OAuth server returns `{needs_auth: true, auth_url}`: sign-in waits on the user, so ask them to finish it in Settings → MCP and call `connect` again afterwards — do not retry while waiting.

`console.mcp.disconnect({id})` — close a server's live session without touching its config.

`console.mcp.reset_auth({id})` — drop a server's stored credential so the next connect signs in afresh.

`console.mcp.call({id, tool, args?})` — run one tool on a server directly, connecting first if needed. Prefer the server's real tools (available through `loadTools`) for ordinary use; `call` is for batching many remote calls in one turn. Returns `{content, is_error}`.


