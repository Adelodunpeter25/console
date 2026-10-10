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
- `WorktreeService` (`worktree_service.go`) already runs git commands that
  create directories, but it is scoped to worktrees and is explicitly *not*
  wired to routes or sessions yet (per its own header comment).
- The desktop has no create-project dialog of any kind.
- Git auth for private repos: token stored by
  `internal/providers/github` (`LoadCredential`/`SaveCredential`; the file
  resolves via `utils.GitHubCredentialsPath()`, overridable with
  `GITHUB_CREDENTIALS_PATH`), injected as a `GIT_CONFIG_*`
  credential-helper env. Public clones need no auth.
  **This shipped already** — `internal/gitconfig.Configure()` injects three
  `GIT_CONFIG_*` triples at boot (called first in `serve.Run`, before the first
  PTY spawn): the `credential.helper` shell command plus two
  `url.https://github.com/.insteadOf` rewrites covering `git@github.com:` and
  `ssh://git@github.com/`. So the "SSH-style URL pastes clone over HTTPS"
  requirement needs **no new code** — any git child that inherits `os.Environ()`
  gets it for free, which is exactly what `runGit` does.
- Repo shape the implementation must fit: the projects domain is
  **proto/protojson-migrated**. `routes/projects.go` builds a `{success, data}`
  envelope where `data` is `protojson.Marshal` output, and payloads are pinned
  to golden fixtures under `proto/testdata/projects/`. New request bodies
  therefore follow the proto path, not plain `c.BodyParser` structs.

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
- Clone takes a while: the dialog shows a busy state and disables its inputs
  for the duration of the request. **No streamed `--progress` output in v1** —
  the server call is synchronous (§3.0), so a spinner is the honest
  representation. Cancellation is also deferred: with no job handle there is
  nothing to cancel against.
- Out of scope: creating a *new* GitHub repo from console (the dialog clones
  or starts blank locally only), repo browsing/picking from a list, templates.

## 3 Server Design

### 3.0 Resolved decisions (were Open Questions — see §8)
- **Where the code lives**: a new `ProjectBootstrapService`
  (`internal/services/project_bootstrap.go`), **not** `GitService`.
  `worktree_service.go` deliberately drew that line — its header states "git
  stays status/diff/branches/checkout, worktrees stay in this service" — and
  clone/init are exactly the "create directories on disk" category it carved
  out. Widening `GitService` would contradict a boundary the codebase already
  documented.
- **Progress mechanism**: **synchronous request + client busy state. No
  streaming in v1.** There is no existing streaming-clone plumbing to reuse:
  `runGit` is capture-only (`bytes.Buffer` stderr, returns on exit), and
  `BashJobManager` is a session-scoped agent-tool subsystem with job ids,
  retention windows, and kill semantics — a poor fit for a request-scoped clone
  that has exactly one caller. Streaming would mean building job lifecycle
  plumbing for a single-user dialog that a spinner covers fine. Re-open only if
  clone times start to matter in practice.
- **Wire format**: new proto messages in `proto/console/v1/project.proto`
  (`InitProjectRequest`, `CloneProjectRequest`) + `make proto` + golden
  fixtures, matching the already-migrated projects domain.
- **Missing-credential handling**: check `github.LoadCredential()` *before*
  spawning git and return the "connect in Accounts" error up front, rather than
  string-matching git's stderr after the fact. Git's message for an unauthed
  private clone (`could not read Username for 'https://github.com'` / HTTP 403)
  is inherently brittle to match, and `WorktreeService.DeleteBranch` shows the
  codebase does substring-match git output — but only where there is no
  alternative. Note the ordering constraint this preserves: a public repo with
  no token must still succeed, so a missing token is only fatal for hosts we
  cannot prove are public.

### 3.1 `ProjectBootstrapService`
- `InitBlank(parentDir, name)` → `os.MkdirAll(filepath.Join(parentDir, name))`
  then `git init`. No initial commit in v1 (matches "blank git repo" semantics;
  the agent can commit when there is something to commit).
- `Clone(url, parentDir, name)` → resolves the target dir (default `name` from
  the URL's last path segment), runs `git clone --progress <url> <dir>`. Reuse
  the package's `runGit` so stderr reaches the error string; no special env
  plumbing is needed because `gitconfig.Configure()` already put the
  credential helper and SSH→HTTPS rewrites into `os.Environ()`, which `runGit`
  children inherit.
- Target-dir guard, shared by both: if the target exists and is non-empty,
  refuse and say so. Never overwrite.
- Typed sentinel errors in the style of `worktree_service.go`
  (`ErrNotGitRepo`, `ErrWorktreeDirty`): `ErrTargetNotEmpty`, plus an
  `ErrGitHubNotConnected` carrying the "connect in Accounts first" wording the
  dialog renders.
- Full clone, no `--depth` (see §8).

### 3.2 Routes (extend `apps/server-go/internal/routes/projects.go`)
- `POST /api/projects/init` — `InitProjectRequest{name, parent_dir}`.
- `POST /api/projects/clone` — `CloneProjectRequest{url, parent_dir, name?}`.
- Both validate the parent dir exists and return the registered `ProjectInfo`
  through the existing `sendProject` protojson helper, so the dialog gets the
  identical payload shape `POST /api/projects` already returns.
- Errors come back as 4xx/5xx `{"success":false,"error":"..."}` messages the
  dialog displays verbatim: 400 validation/bad URL/`ErrTargetNotEmpty`,
  409 for "not connected" (distinct so the client can point at Accounts rather
  than treating it as a typo), 502 for network/unreachable.
- `RegisterProjectRoutes` gains a `*services.ProjectBootstrapService` parameter;
  `routes.New` (`routes.go:68`) constructs and passes it.
- `POST /api/projects` (register-existing-path) stays exactly as is.

## 4 Desktop Changes
- New "New project" dialog (name + parent folder + two option cards), reachable
  from the same places "add project" lives today (settings project list, ⌘O
  browser). Calls the two new endpoints, shows a busy state during the call,
  opens the resulting project on success.
- The Accounts GitHub item itself (connect / PAT / status) belongs to the
  credentials plan — this dialog only *reads* its status to decide whether to
  prompt for connection first.

## 5 Implementation Steps
- 1: proto — `InitProjectRequest` / `CloneProjectRequest` in
  `project.proto`, `make proto`, golden fixtures under
  `proto/testdata/projects/`.
- 2: `ProjectBootstrapService.InitBlank` + `Clone` with tests
  (`apps/server-go/tests/services/`): blank creates a dir with `.git`;
  clone into a local origin succeeds; existing non-empty target refused; bad
  URL; SSH-style URL rewrites to HTTPS (reusing the `runGitInherit` /
  `gitconfig.Configure` helpers from `tests/services/gitconfig_test.go`);
  no-token + private URL returns the Accounts-pointing error.
- 3: the two routes with the same coverage at the HTTP layer
  (`apps/server-go/tests/api/`), including the 409-not-connected case and the
  400 for an existing non-empty target.
- 4: desktop client methods on `console-core`'s `ProjectService`
  (`src/services/project.rs`) next to the existing `list`/`add`/`remove`.
- 5: desktop dialog — blank card first (no auth dependency), then clone card
  with busy state + not-connected → Accounts handoff.
- 6: docs: dialog copy, public-vs-private behavior note, "connect in Accounts
  first" path.

Test conventions being followed: Go tests live in `apps/server-go/tests/<area>/`
(package `tests`), never inline in source files; service tests drive the real
`git` binary with `GIT_CONFIG_NOSYSTEM=1` and an isolated `HOME` (see
`tests/worktrees/worktree_service_test.go:20`); route tests build a bare
`fiber.New()` + `RegisterXRoutes` against an in-memory DB (see
`tests/api/projects_test.go:65`).

Per AGENTS.md, only the specific test files relevant to a step get run — not
the full suite.

## 6 Verification
- Blank: name + folder → folder exists with `.git`, project opens, terminal works in it.
- Public clone with no token stored: succeeds, never prompts for auth.
- Private clone with no token: clear error pointing at Accounts; connect (PAT),
  retry same dialog → succeeds.
- Private clone with token: succeeds with no prompt; `git pull` in the project
  terminal works after.
- Target dir exists and non-empty: refused with a clear message, nothing overwritten.
- SSH-style URL pastes clone over HTTPS — via the existing `gitconfig`
  `insteadOf` entries, so this is a regression guard, not new behavior.
- Daemon restart mid-nothing (no state to lose — clone is synchronous per request).

## 7 Out of Scope
- Creating new GitHub repos from console (clone or blank-local only).
- Repo list browsing / picking (paste-a-URL only in v1).
- Project templates / scaffolding of any kind.
- Worktrees (clone once per project, then one worktree per session; explicitly
  not tangled in — `WorktreeService` already exists and stays untouched).

## 8 Open Questions
- ~~Clone progress mechanism~~ — **resolved**: synchronous + busy state, no
  streaming, no cancellation in v1 (§3.0).
- ~~Where clone/init live~~ — **resolved**: new `ProjectBootstrapService`,
  keeping `GitService` scoped to status/diff/branches/checkout (§3.0).
- ~~Request body format~~ — **resolved**: proto messages, matching the
  already-migrated projects domain (§3.0).
- ~~Shallow vs full clone default~~ — **resolved: full.** Shallow clones are
  painful to repair later (a `git fetch --unshallow` the user must know to run,
  and history-dependent agent work breaks). Revisit only if someone hits a
  genuinely huge repo and complains.
- Default parent folder: remember last-used, or a fixed `~/Developer/Projects`-style
  default? Still open — desktop-side, cheap to decide at step 5.
