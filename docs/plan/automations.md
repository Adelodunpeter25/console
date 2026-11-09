# Automations — an agent that schedules its own future work

The agent can promise future work and keep the promise. "Do this and that every
5 minutes", "merge this change, then remind me in two days to remove the
feature flag and roll it out to everyone" — the agent does the first half now
and schedules the second half, on the same thread, with the thread's full
context. Shape follows Amp's automations
(https://ampcode.com/docs/orbs/automations); the machinery is Console's own.

Tool surface: four namespaced tools the model calls like any other tool —

- `console.automation.create(prompt, schedule, ...)` — one-shot or recurring
- `console.automation.get(id)`
- `console.automation.update(id, ...)`
- `console.automation.delete(id)`

One schedule per session. A second automation is a new thread's job, not a
second row on this one.

---

## 0. Decisions (settled)

- **Namespaced tool calls, not a pile of tools.** `console.automation.*` is the
  surface; internally it lands in one tool file with four verbs, the way
  `internal/agent/tools` groups the bash tools.
- **Natural language in, structured out.** The agent parses "every 5 minutes",
  "in two days", "weekdays at 8:30" into a typed schedule (`interval` /
  `at` / one-shot `run_at`). Cron is never exposed to the user or the model;
  the stored record is machine-typed so the scanner can compute next_run.
- **Completion conditions are prompt-driven, not code.** "Stop checking when
  the backfill finishes" means the agent calls `console.automation.delete`
  when it observes done. The harness does not try to understand the task.
- **One-shot reminders stay in the same thread.** The whole point of "remind
  me in two days to remove the feature flag" is that the reminder wakes up
  knowing which flag, which PR, which rollout. A fresh thread would lose that.
- **No minimum-interval floor in v1.** If the agent schedules something every
  5 seconds, that is a prompt-quality bug, fixed in the prompt and skill text,
  not with a harness-side clamp.
- **The desktop app is just a client.** Schedules live on the server. A
  schedule fires whenever the server is up, whether or not any desktop app is
  connected; the run appears in the thread like any other and is there when
  the user reopens the client. There is no "desktop must be running" mode and
  no separate cloud stage — the server is the cloud.

## 1. Non-Goals (v1)

- **No webhooks / event triggers.** Amp's webhook story is a plugin surface
  waking an orb; ours would be a relay endpoint waking a session. Defer until
  a real external source (GitHub CI, incident tool) asks for it.
- **No UI to author schedules.** Users author by talking. A read-only
  indicator on scheduled threads (title, schedule, next run, prompt) is the
  only UI cost in v1.
- **No cross-session schedules.** An automation belongs to the session that
  created it and wakes that session.
- **No cron expressions.** Typed schedule structs only; the model translates.

## 2. Pieces

- **`session_schedules` table** (`internal/db/schema.go`), next to
  `session_queue`: id, session_id, prompt, schedule (typed), next_run_at,
  status (active/paused/done), created_at. Persisted, so schedules survive
  server restarts.
- **Schedule manager** (`internal/services/` or `internal/run/`): a scanner
  that ticks, finds due `active` schedules, computes `next_run_at` for
  recurring ones (or flips one-shots to `done`), and enqueues.
- **Wake = enqueue.** A fire is `QueuePrompt` into the owning session with the
  saved prompt, tagged so the transcript shows it as a scheduled run. The run
  package's `startStagedIfIdle` already handles the idle/steering cases; if a
  run is active, the fire waits its turn in the queue.
- **Tools** (`internal/agent/tools/automation_tool.go`): the four verbs,
  registered in the registry like any other tool, writing through the db.
- **Prompt/skill text**: how the model translates natural-language timing into
  the typed schedule, how a monitoring prompt is written (what to inspect,
  what to report, when to stop), and that the agent should delete its own
  schedule on completion.

## 3. Sequence

1. **Stage 1 — one-shot + recurring schedules, same session, server-resident.**
   Create/get/update/delete, persisted, fired by the scanner into the prompt
   queue. This alone covers every example above.
2. **Stage 2 — scheduled-thread indicator** in the client: next run, schedule,
   saved prompt, pause/resume.
3. **Stage 3 — event triggers via relay**, when something external needs to
   wake a session.

## 4. Verification

- Unit: schedule math (interval, at, one-shot completion), scanner firing into
  the queue, restart persistence.
- End to end: create a 1-minute recurring automation in a test session, watch
  two fires land as queued prompts, delete it, confirm no third fire.
- A one-shot created in a session fires once, marks done, and the run's
  transcript shows the saved prompt with thread context intact.
