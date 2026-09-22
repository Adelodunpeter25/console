# New-Project Dialog Plan (Blank + Clone from GitHub)

Goal: a desktop "New project" dialog with exactly two options — **blank
project** and **clone from GitHub** (public or private). No templates, no
quick-start scaffolding. This breaks the bootstrap catch-22: the desktop
scopes terminals/sessions to a project, so a repo that only exists on GitHub
currently cannot be brought onto the box without a terminal — cloning becomes
a server-side operation launched from the dialog itself.

## 1 Current State
- Projects are only ever *registered* from an existing local path: desktop
  `add_project_from_path` (⌘O browser / native folder picker, via
  `open_project_browse` in `apps/desktop/src/settings_window.rs`) → `POST
  /api/projects` → DB row. Nothing creates folders.
- `ProjectService.Create` (`apps/server-go/internal/services/project_service.go`)
  only inserts/updates the `projects` row (`id, name, dir`) — no mkdir, no
  `git init`, no clone.
- `GitService` (`apps/server-go/internal/services/git_service.go`) does
  status/diff/branches/checkout only — no clone, push, or pull.
- The desktop has no create-project dialog of any kind.
- Git auth for private repos comes from the credentials plan
  (`docs/plan/github-git-credentials-plan.md`): token in
  `~/.console/github-creds.json`, injected via `GIT_CONFIG_*`
  credential-helper env. Public clones need no auth.

## 2 UX Proposal
- Desktop "New project" dialog, two cards, no templates:
  - **Blank**: project name + parent folder → server creates the folder,
    `git init`, registers the project, opens it. No auth needed, never blocked.
  - **Clone from GitHub**: repo URL field (HTTPS `https://github.com/org/repo`
    or SSH-style `git@github.com:org/repo`, the latter rewritten to HTTPS per
    the credentials plan) + parent folder → server clones with the stored
    token, registers the project, opens it. Folder name defaults from the URL;
    target dir must not already exist (or must be empty).
- **Not-connected handling**: if no GitHub token is stored, the clone option
  is not dead — it points at Accounts to connect first ("Connect GitHub in
  Accounts, then come back"), then continues. Public-URL clones never require
  auth and are never blocked by missing credentials.
- Clone takes a while: the dialog shows progress (at minimum a busy state;
  ideally streamed `git clone --progress` output) and stays cancellable.
- Out of scope: creating a *new* GitHub repo from console (the dialog clones
  or starts blank locally only), repo browsing/picking from a list, templates.

## 3 Server Design
- `GitService.Clone(url, dir)` in `git_service.go`: runs
  `git clone --progress <url> <dir>` with the same `GIT_CONFIG_*`
  credential-helper env as the credentials plan (private repos authenticate
  silently; public repos ignore it). Long-running: report progress — reuse
  whatever mechanism the codebase already has for streaming child output
  (e.g. the bash-job output pattern), no new infra if avoidable.
- `GitService.InitBlank(dir)`: mkdir (parents included), `git init`,
  nothing else. No initial commit in v1 (matches "blank git repo" semantics;
  the agent can commit when there is something to commit).
- Routes (extend `apps/server-go/internal/routes/projects.go`):
  - `POST /api/projects/init` — `{name, parentDir}` → blank project.
  - `POST /api/projects/clone` — `{url, parentDir, name?}` → cloned project.
  - Both return the registered `ProjectInfo`; both validate the parent dir and
    refuse to overwrite a non-empty target. Errors (bad URL, auth failure,
    network) come back as plain 4xx/5xx messages the dialog can display —
    auth failures should name Accounts as the fix.
- `POST /api/projects` (register-existing-path) stays exactly as is.

## 4 Desktop Changes
- New "New project" dialog (name + parent folder + two option cards), reachable
  from the same places "add project" lives today (settings project list, ⌘O
  browser). Calls the two new endpoints, shows clone progress, opens the
  resulting project on success.
- The Accounts GitHub item itself (connect / PAT / status) belongs to the
  credentials plan — this dialog only *reads* its status to decide whether to
  prompt for connection first.

## 5 Implementation Steps
- 1: `GitService.Clone` + `InitBlank` with tests (success, existing-dir
  refusal, bad URL, auth-failure message when no token stored).
- 2: `POST /api/projects/init` + `POST /api/projects/clone` routes with the
  same coverage at the HTTP layer.
- 3: desktop dialog — blank card first (no auth dependency), then clone card
  with progress + not-connected → Accounts handoff.
- 4: docs: dialog copy, public-vs-private behavior note, "connect in Accounts
  first" path.

## 6 Verification
- Blank: name + folder → folder exists with `.git`, project opens, terminal works in it.
- Public clone with no token stored: succeeds, never prompts for auth.
- Private clone with no token: clear error pointing at Accounts; connect (PAT),
  retry same dialog → succeeds.
- Private clone with token: succeeds with no prompt; `git pull` in the project
  terminal works after.
- Target dir exists and non-empty: refused with a clear message, nothing overwritten.
- SSH-style URL pastes clone over HTTPS.
- Daemon restart mid-nothing (no state to lose — clone is synchronous per request).

## 7 Out of Scope
- Creating new GitHub repos from console (clone or blank-local only).
- Repo list browsing / picking (paste-a-URL only in v1).
- Project templates / scaffolding of any kind.
- Worktrees (clone once per project, then one worktree per session — see
  `docs/plan/worktrees-plan.md`; explicitly not tangled in).

## 8 Open Questions
- Clone progress mechanism: stream `git clone --progress` output, or simple
  busy state + completion? (Prefer streaming only if the plumbing already exists.)
- Shallow vs full clone default for large repos?
- Default parent folder: remember last-used, or a fixed `~/Developer/Projects`-style default?
