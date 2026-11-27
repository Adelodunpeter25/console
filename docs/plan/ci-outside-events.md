# Plan: Amp-style "CI subscription" for Console

## Goal

Amp's Orbs wake on outside events (CI failing on main, PR reviews, Linear issues)
via their hosted backend. Console can achieve the same behaviour without any
webhook URL to paste: a server-side watcher notices a new outside event and wakes
the session in its own thread with full context — the same delivery path
automations and webhooks already use.

First event: **CI runs on GitHub**. The same shape extends to PR reviews/issues.

## Why no CLI dependency

There is already a first-party GitHub integration in
`internal/providers/github/`: a stored PAT (`credentials.go`), a REST client
that calls `api.github.com` directly with bearer auth (`pat.go`), and the
credential helper that feeds git. A watcher reuses `github.LoadCredential()`
and plain `net/http` against `GET /repos/{owner}/{repo}/actions/runs`
(plus `/actions/runs/{id}/jobs` for failure details). No shelling out to
`gh`, no new auth flow, no provider setup for the user beyond the GitHub token
Console already supports.

## What already exists (verified)

- `internal/services/automations/` — one active schedule per session, global DB,
  scanner ticks every 15s (`internal/run/automations.go`).
- `internal/services/webhooks/` + `internal/run/webhooks.go` — incoming HTTP
  events wake a session with `source: "webhook"`; requires the sender to POST,
  which is exactly what we want to avoid.
- `s.promptSessionWithSource(...)` in `internal/run/` is the shared wake
  primitive, tagged by source, tagged turns excluded from user history.
- `internal/providers/github/` — stored credential + REST auth headers
  (`Authorization: Bearer`, `Accept: application/vnd.github+json`,
  `X-GitHub-Api-Version: 2022-11-28`, `User-Agent`). `APIBase` is a var so
  tests can point it at an httptest server.
- Repo origin: `https://github.com/Adelodunpeter25/console`.
- Docs: `docs/plan/automations.md` already describes the "subscribe to CI
  results and fix any failure" aspiration.

## Design

Add a **watcher** abstraction alongside automations/webhooks: a server-side
poller with per-session subscriptions, deduplicated by an external cursor so it
wakes on *new* events only, and reuses `promptSessionWithSource`.

### 1. Server: watcher service (`internal/services/watchers/`)

- `Subscription` rows in the global DB: `id`, `session_id`, `kind`
  (initially `github_ci`), `config_json` (repo, branch, workflow),
  `prompt`, `cursor` (last seen run id / created-at), `status`,
  `last_error`, timestamps. Follow the `webhooks.Service` pattern:
  `Create/Get/List/Delete`, `SetStatus`, `AfterFire`, in-process `Event`
  pub/sub for SSE. Schema in `internal/db/schema.go`; cleanup on session
  delete alongside automations/webhooks.
- `github_ci` poll logic in `internal/services/watchers/github.go`: use
  `github.LoadCredential()` for the token, build a request with the same
  headers as `github.ValidatePAT`, call
  `GET {APIBase}/repos/{owner}/{repo}/actions/runs?branch=<branch>&per_page=20`,
  decode the JSON into a small struct (id, name, status, conclusion,
  created_at, html_url, head_branch). Compare each run against the stored
  cursor; select *newly completed* runs. Run ids increase, so "last seen run
  id" is enough. On failure, optionally fetch
  `/actions/runs/{id}/jobs?filter=failed` for log URLs / failed step names to
  include in the wake prompt.
- Poller (`internal/run/watchers.go`, mirroring `automations.go`):
  `StartWatcherScanner` ticks every 15–30s, iterates active subscriptions,
  re-checks status inside the loop, wakes the owning session with
  `promptSessionWithSource(sessionID, promptWithEventDetails, "watcher", ...)`,
  persists the new cursor with `AfterFire`, applies the same transient-failure
  retry/backoff. Never wakes twice for the same run id. Reuse the existing
  HTTP client/timeouts pattern from `pat.go`; log errors without the token.
- Wire into `routes.go`: `watchers.New(cfg.DB.Global())`,
  `runSvc.SetWatchers(...)`, register management routes. Start/stop scanner
  with the automation scanner.

### 2. Agent tool surface (`internal/services/console/`)

Register in `consoleCatalog()` (`registry.go`), following the
`console.webhooks.*` shape:

- `console.watchers.subscribe({kind: "github_ci", repo, branch?, workflow?, prompt?, once?})` — auto-detects repo from `git remote -v` / current dir if omitted. Returns subscription id, cursor, and a human-readable description.
- `console.watchers.list()` / `.get({id})` / `.unsubscribe({id})`
- Default prompt when omitted: "CI run <id> for <repo> finished with <conclusion>. Check the failing jobs/logs and fix the cause, then verify." The poller appends event details.

### 3. Client UI (minimal)

- Proto/JSON model + SSE stream `GET /api/watchers/stream` and management
  routes `GET/POST/DELETE /api/watchers` mirroring automations/webhooks.
- Sidebar/composer status card ("Watching CI on owner/repo", last event), with
  pause/resume/delete. Mark watcher-originated turns with a badge in desktop and
  Android (the `source` TODO already in `docs/plan/automations.md`).

### 4. Verification

- Go tests in `internal/services/watchers/` (cursor dedupe, failure-only
  filter, request headers/URL using httptest + `github.APIBase` override) and
  `internal/run/watchers_test.go` (wake once per event, cursor advance,
  transient error retry), run only those files per AGENTS.md.
- Manual check: subscribe to `github_ci` on this repo, push a commit, confirm
  exactly one tagged turn after the run completes; confirm no wake on later
  polls.

## Interim workaround (no code)

An interval automation can check CI via the GitHub API (or `gh`) inside the
agent turn and self-delete when done. Works today, but burns a turn per poll
and has no cursor/dedupe. Good for a demo, not the product answer.

## Scope notes

- Start with GitHub CI via the existing PAT provider; GitLab/Linear/PR reviews
  are follow-ups with the same subscription/wake pattern.
- Keep the inbound webhook service for senders that do POST; Amp-style just
  means users should not have to configure it.
