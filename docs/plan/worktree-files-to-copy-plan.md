# Worktree Files-to-Copy Plan

Goal: every worktree Console creates carries the gitignored local files it
needs to work (signing keys, `.env*`, local config), so a fresh worktree
behaves like the checkout it came from. Modeled on Conductor's "Files to
copy" (`conductor.build/docs/reference/files-to-copy`).

Motivating instance: `apps/android` debug builds sign with the release key
only when `apps/android/key.properties` exists. That file and
`keystore/console-release.keystore` are gitignored, so fresh worktrees fall
back to Gradle's auto-generated debug key: the build "succeeds" but
`installDebug` fails with `INSTALL_FAILED_UPDATE_INCOMPATIBLE` while the
device keeps running the old build. The fix below is generic; Android is
just the first entry.

## 1 Resolution order

For a given source repo, patterns come from the first source that exists:

1. `.worktreeinclude` at the repo root (committed, tool-agnostic — Claude
   Code reads the same file, so manual CLI worktrees via other tools keep
   working).
2. Default: `.env*` (Conductor parity).

No settings UI in v1; the `.worktreeinclude`-wins rule reserves that slot
for a future per-project `file_include_globs`.

## 2 Pattern syntax

Gitignore syntax (blank lines, `#` comments, `!` negation, trailing `/`
dir-only, `**`), plus one Console extension:

- plain line → **copy** (snapshot at creation; per-worktree edits stay
  isolated; survives source deletion),
- `link:` prefix (e.g. `link: apps/android/key.properties`) → **symlink**
  (live; key rotation propagates to all worktrees).

Eligibility (Conductor parity): only files that are gitignored in the
source checkout are carried over — checked via `git check-ignore`, never
by reimplementing ignore semantics. Tracked files are already in the new
worktree; non-ignored untracked files are never copied. Dependency/build
dirs (`node_modules`, `build`, `.gradle`, `target`, `dist`) are skipped
even if matched, with a warning — they are large, regenerable, and carry
stale state.

## 3 Copy engine + hook

- New `apps/server-go/internal/services/worktree_files.go`: resolve
  patterns, enumerate gitignored matches in the source checkout, copy
  (preserving relative paths and modes, creating parent dirs) or symlink
  `link:` entries, skipping entries whose source is absent (e.g. a machine
  with no release key). No glob/gitignore lib in `go.mod` today; match via
  a small added dep (e.g. doublestar for `**` + `!`/trailing-`/` rules) or
  `git ls-files` pathspecs — implementer's choice, contained in the new
  file. Never log file contents; `key.properties` holds passwords.
- Hook in `WorktreeService.WorktreeAdd`
  (`apps/server-go/internal/services/worktree_service.go:71`), which covers
  both session-create and attach paths via `provisionWorktree`
  (`session_service.go:154`) — the single choke point for worktrees Console
  creates.

## 4 Failure semantics

Carry-over is strictly best-effort: any failure copies/links nothing extra
and worktree creation proceeds exactly as before. No error surfaces to the
caller; at most a debug-level log for diagnosability. A failed carry-over
must be indistinguishable from today's behavior.

## 5 Seed this repo

- Commit root `.worktreeinclude`:
  ```
  .env*
  link: apps/android/key.properties
  link: apps/android/keystore/**
  ```
  `storeFile` in `key.properties` is relative to `apps/android`, so linking
  the `keystore` dir keeps it resolving.
- Add `keystore/` to `apps/android/.gitignore` (today only `*.keystore` is
  covered, which is why linked/copied keystores need a per-worktree
  `info/exclude` entry). Committed once, works in every worktree.
- `Makefile:dev-mobile` and `console.toml:dev-mobile` abort with an
  actionable message when `key.properties` is absent — backstop for
  hand-run `git worktree add`, which bypasses the server hook. Verify via
  `cd apps/android && ./gradlew :app:signingReport -q`: `debug` must show
  `Config: release`, not `Config: debug` or `~/.android/debug.keystore`.

## 6 Verification

- New tests in `apps/server-go/tests/worktrees/` (dedicated files, no
  inline test modules): temp repo with committed `.worktreeinclude` plus
  gitignored, tracked, and plain-untracked files → `WorktreeAdd` copies
  only the gitignored matches, symlinks `link:` entries, skips missing
  sources, and a failing carry-over still leaves a usable worktree (§4).
  Run only that file: `cd apps/server-go && go test ./tests/worktrees/ -run <TestName> -v`.
- Manual: fresh worktree in this repo → `signingReport` shows `Config:
  release` for `debug`; `installDebug` updates the existing
  `com.console.mobile.dev` install.

## 7 Out of scope

- Hand-run `git worktree add` (bypasses the server; covered by the
  committed `.worktreeinclude` + a future CLI wrapper reusing the same
  file, not here).
- Settings UI / per-project `file_include_globs`.
- Copying dependency or build output; setup scripts own those (Conductor
  §"What gets copied").
