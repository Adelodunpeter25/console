# Worktrees Plan (one worktree per session)

Goal: parallel sessions on one repo without branch-stomping. A session can
optionally live in its own git worktree + branch, so two agents (or agent +
human) work the same project without touching each other's files. Thin on
purpose: this is not a worktree manager — creation, scoping, and above all
cleanup discipline are the whole feature.

## 1 Current State
- Sessions are scoped to a `cwd` (`SessionHeader.Cwd`,
  `apps/server-go/internal/types/session.go`); everything downstream — agent
  tools, assist search, fff prewarm — keys off that string, so a worktree dir
  works transparently as a session root with zero changes to consumers.
- `UpdateCwd` + the cwd lock (no project/cwd moves after the first message,
  `apps/server-go/internal/routes/sessions.go`) already pins a session to one
  directory for its lifetime — the exact invariant a worktree needs.
- `GitService` (`apps/server-go/internal/services/git_service.go`) does
  status/diff/branches/checkout via `runGit` — no worktree ops.
- `fswatch` already resolves worktree `.git` pointer files so commits recorded
  outside the worktree dir still notify the right view
  (`apps/server-go/internal/services/fswatch_service.go`) — file watching is
  worktree-safe today.
- Session lifecycle exists: create, soft-delete, restore, permanent delete
  (`SessionService`, `apps/server-go/internal/services/session_service.go`).
- Assumes a cloned project dir exists — i.e. this comes after the credentials
  plan (auth for private remotes) and the new-project dialog plan (clone once
  per project): `docs/plan/github-git-credentials-plan.md`,
  `docs/plan/new-project-dialog-plan.md`.

## 2 UX Proposal (after backend — server first, UX later)
- New-session flow offers "Isolate in worktree" (**default off, always**):
  server creates `<branch>` + worktree, session cwd points at it.
  Branch name auto-derived from the session title (slug + short id), editable.
- The session otherwise looks and behaves identically — same terminal, same
  agent, same @-mention search. The only visible difference: the session shows
  which branch/worktree it owns.
- Cleanup is the feature: deleting a session (permanent delete) removes its
  worktree. Dirty worktree (uncommitted changes) blocks removal with a clear
  message — never silently destroy work. Orphaned worktrees (session row gone,
  worktree dir remains, e.g. after a crash) surface in one minimal list with
  remove/prune actions; no full manager UI.
- Out of scope: merge/rebase UI, PR creation, worktree-vs-worktree diffing,
  bare repos, multiple worktrees per session.

## 3 Server Design
- New `WorktreeService` in its own file
  (`apps/server-go/internal/services/worktree_service.go`) — worktree code
  does NOT live in `GitService`/`git_service.go`. Modularity: git stays
  status/diff/branches/checkout, worktrees stay in the worktree service
  (reuses the same git-running helper, owns nothing else).
  - `WorktreeAdd(repoDir, path, branch)` — `git worktree add -b <branch> <path>`.
  - `WorktreeList(repoDir)` — `git worktree list --porcelain`, parsed.
  - `WorktreeRemove(repoDir, path, force)` — refuses when dirty unless forced.
  - `WorktreePrune(repoDir)` — `git worktree prune` for stale metadata.
- Worktree location (decided): central `$HOME/console/worktrees/<id>`
  (non-hidden `console` dir in the user's home — not `~/.console`, not
  in-repo). Keeps repos clean; one known root for orphan recovery.
- Refuse unborn HEAD: a repo with no commits yet cannot back a worktree
  (`git worktree add -b` has no base) — creation fails with a clear error
  telling the user to commit first. Not supported in v1.
- Session create accepts an optional worktree spec `{branch?}`: when present,
  create branch + worktree first, then set `header.cwd` to the worktree dir.
  Failure rolls back (remove the worktree) so a half-created session never
  lingers.
- Session permanent delete: if the session owns a worktree, remove it
  (blocked when dirty — surface the error, keep the session). The branch is
  always left in place in v1 (leave-always cleanup policy — branches
  accumulate, deletion stays explicit).
- Dirty check before removal: `git status --porcelain` in the worktree must be
  empty (untracked files count — an agent's half-written work is still work).

## 4 Client Changes
- New-session UI: worktree checkbox + branch-name field (prefilled).
- Session header/row: worktree + branch indicator.
- One orphan list (project settings or session list overflow): worktree path,
  branch, dirty/clean, remove action. Nothing fancier in v1.

## 5 Implementation Steps (server only first — §4 client/UX after backend lands)
- 1: `WorktreeService` worktree ops with tests (add/list/remove/prune, dirty
  refusal, unborn-HEAD refusal, rollback on create failure).
- 2: session create worktree spec + permanent-delete cleanup.
- 3 (later): client: new-session checkbox + indicator + orphan list.
- 4: docs: branch naming, dirty-block behavior, orphan recovery.

## 6 Verification
- New session with worktree: branch exists, worktree dir exists, session cwd
  points at it, agent edits land on the branch, main checkout untouched.
- Two sessions on one project: independent files, independent branches.
- Delete clean session → worktree gone, branch remains, `git worktree list`
  clean.
- Delete dirty session → blocked with a clear message, work intact.
- Kill daemon mid-session, restart → orphan appears in the list, remove works.
- fff search + @-mention + terminal all work with the worktree dir as root.

## 7 Out of Scope
- Merge/rebase/PR flows (use the terminal or GitHub; the branch is just there).
- Bare-repo or `--detach`ed worktrees.
- More than one worktree per session.
- Auto-deleting branches (explicit only, see below).

## 8 Decisions (locked for v1)
- Worktree location: central `$HOME/console/worktrees/<id>`. Decided.
- Branch cleanup: leave always. Decided.
- Default: off always. Decided.
- Unborn HEAD (repo with no commits): refused with a clear error. Decided.
