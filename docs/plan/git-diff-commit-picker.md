# Git-first diffs with a commit picker

## Problem

The Changes surfaces show a per-session, per-turn diff recorded by the run service (`GET /api/sessions/:id/changes` and `/changes/diff`). With codemode active, the agent writes files through the JavaScript REPL instead of the recorded `writeFile` / `editFile` tools, so the session diff no longer reflects what actually changed. Git is now the source of truth: working-tree changes and committed history are real, complete, and independent of which tool path produced them.

Users need to:

1. Choose whether the Changes surface shows **git** diffs or the legacy **session** diff.
2. Pick a commit from a dropdown and see that commit's changes — all files first, then one file's patch.

## Decisions

- **Git is the default source**, session diff remains available as a fallback for older chats and non-codemode runs.
- **The commit picker is global** to the Changes screen. Selecting a commit replaces the file list with the files changed in that commit; clicking a file shows only that file's patch. "Working tree" is always the first entry.
- GitHub Enterprise / other hosts remain out of scope — this is local git history only.
- The commit picker lists commits reachable from `HEAD` in the session's project checkout (the same repo path already used by `/api/git/status`). No cross-repo browsing.
- No new diff renderer. Desktop `DiffViewer` and Android `DiffView` already take unified diff text; this feature is plumbing + UI chrome.

## Server

### 1. Git service (`internal/services/git/git_service.go`)

- `ListCommits(repoPath string, limit int) ([]types.GitCommit, error)`: run `git log --max-count=<limit> --date=iso-strict --format=%H%x1f%h%x1f%ad%x1f%an%x1f%s`. Parse each line on `0x1f`. Reject a non-positive or out-of-range limit (clamp to 1…100). Return empty on not-a-repo rather than an error, matching `GetGitStatus`.
- `GetCommitDiff(repoPath, sha, filePath string) (string, error)`: validate `sha` is 4–64 hex characters (reject anything else so a ref name cannot turn into a git flag), then run `git show --format= --no-ext-diff <sha>` or `git show --format= --no-ext-diff <sha> -- <filePath>`. Quote the file path as an argument to `exec.Command`, not by string concatenation.
- `GetCommitFiles(repoPath, sha string) ([]GitFileEntry, error)`: run `git show --format= --name-status <sha>` (or `diff-tree`) and map statuses to the existing `GitFileEntry` shape; additions/deletions can be fetched with `git show --numstat` in the same call if cheap, or omitted (optional fields) for the first cut.
- Keep all new commands on `RunGit` / `runGitCtx` so stderr reaches the caller.

### 2. Proto (`proto/console/v1/git.proto`)

Add (canonical omissions as elsewhere):

```proto
message GitCommit {
  string sha = 1;
  string short_sha = 2;
  string subject = 3;
  string author = 4;
  string date = 5; // ISO 8601 from git's %ad
}
message GitCommitsResponse {
  repeated GitCommit commits = 1;
  bool is_git_repository = 2;
}
message GitCommitFilesResponse {
  repeated GitFileEntry files = 1;
}
```

Extend `GitDiffResponse` with `optional string commit_sha = 3;` so clients can echo back which commit a diff belongs to.

Regenerate Go protobuf with the local `protoc-gen-go` per `proto/buf.gen.yaml`; desktop/Android generate from the same `.proto` at build time (Wire for Android, prost for Rust) — no checked-in generated code for those clients.

### 3. Routes (`internal/routes/git.go`)

- `GET /api/git/log?path=<repo>&limit=<n>` → `GitCommitsResponse`.
- `GET /api/git/commit-files?path=<repo>&commit=<sha>` → `GitCommitFilesResponse`.
- Extend `GET /api/git/diff`: when `commit=<sha>` is present, call `GetCommitDiff(repoPath, sha, path)`; otherwise keep the current working-tree behavior unchanged. Reject invalid `path` / `commit` combinations with 400.
- All routes stay behind the existing device-token middleware (registered in `routes.New` after `RequireDeviceToken`).

### 4. Types

Add `types.GitCommit` next to `GitStatusSummary` / `GitFileEntry`; keep conversions in `services.GitCommitToProto` mirroring `GitSummaryToProto`.

## Desktop client

- Add a persisted setting `diffSource: "git" | "session"` (default `git`) next to the existing settings state. Expose as a small segmented control / picker in the Changes view header on desktop.
- The Changes surface (workspace tab / right-hand pane) gets a commit dropdown at the top, populated from `GET /api/git/log` for the active session's cwd (fall back to the selected project path). Top entry: `"Working tree"`. Then one row per commit: `<shortSha>  <subject>  <date>`.
- Selecting `"Working tree"` keeps today's behavior: git status file list + `GET /api/git/diff` per file. Selecting a commit calls `GET /api/git/commit-files`, shows that list, and each file loads via `GET /api/git/diff?commit=<sha>&path=<file>`.
- Session source keeps the current `GET /api/sessions/:id/changes` flow untouched.
- Cache: reuse `open_diff_contents` keyed by `(source, commitSha-or-"worktree", path)`; invalidate on commit change and on the existing git status watch events for working tree.
- Wire path: `console-core/src/services/git.rs` gets `log()`, `commit_files()`, and an optional `commit` parameter on `get_diff()`; state lives in `src/state/workspace_tabs/file_and_diff.rs` and/or a small `src/state/changes.rs`.

## Android client

- Add a `DiffSource` preference in the existing settings store (default `GIT`), and a segmented control in `ChangesScreen` header.
- Add `GitRepository` methods in `data/repo/` for `getCommits(repoPath)`, `getCommitFiles(repoPath, sha)`, and `getDiff(repoPath, path, sha?)`, decoding the new proto messages via Wire.
- `ChangesScreen`: a dropdown (`ExposedDropdownMenuBox` or the existing `PickerSheets` pattern) above the file list with `"Working tree"` + commits. Selecting a commit reloads the file list via `getCommitFiles`; clicking a file loads `getDiff` with that `sha`.
- Keep the existing diff cache but key it by `(source, sha, path)`, and clear it when the commit or source changes.
- Reuse `DiffView` and `SharedNative.parseDiff` as today — no diff parser work.

## Tests

- Go: `tests/git/log_test.go` — real temp git repo with 3 commits; assert order, short sha, subject, and that `GetCommitDiff` returns only the selected commit's patch; invalid sha is rejected.
- Go route tests: `/api/git/log`, `/api/git/commit-files`, and `GET /api/git/diff?commit=…` behind auth.
- Proto fixtures: `proto/testdata/git/log.json`, `commit_files.json`, `diff_with_commit.json` decoded by Go, desktop, and Android tests.
- Android: `ChangesScreen`: selecting a commit swaps the list and loads the file diff with the sha (unit test at repository level + manual UI pass).

## Rollout

1. Server + proto + Go tests.
2. Desktop setting + commit picker + wiring.
3. Android setting + commit picker + wiring.
4. Flip default to git in both clients once validated; leave session diff as an explicit choice.

## Out of scope

- Commit search / filtering beyond the recent `limit`.
- Diffing arbitrary commit ranges (A…B) — one commit at a time only.
- Showing commits from remotes not reachable from local `HEAD`.
- Staging, checkout, or mutating git state from the picker.
