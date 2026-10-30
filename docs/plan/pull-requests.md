# Pull requests — read, check status, merge

One GitHub service on the server, used by three surfaces: the agent (`console.pr.*`), the desktop app, and the Android app. Mobile is a first-class client here, not a reduced one.

Scope is **github.com only**. GitHub Enterprise Server is out (same as the git credential helper).

Reference: `/Users/adelodunpeter/Developer/Projects/t3code` has a much larger PR feature (about 30 RPCs, 7 git hosts, stacks, an AI review guide, inline review). It is React Native and TypeScript, so nothing ports directly; it was read as a map of what the feature contains and what to leave out.

---

## 0. Goals and Non-Goals

**Goals**
- See your pull requests (authored by you, or waiting on your review) and the open ones in a project's repo.
- Open one and see what matters: description, checks, review state, mergeability, changed files, comments.
- Act on it: merge, close, reopen, mark ready for review.
- Create and edit a pull request, so a finished agent task can end in one.
- The agent can do all of the above through the `console` kernel, with the approval rules every other write goes through.

**Non-goals (v1)**
- Inline review comments, submitting reviews (approve / request changes), thread replies, resolving threads, reactions. This is the largest piece of t3code's feature and the one that makes it a review tool; it is a separate follow-up, not part of v1.
- Stacked PRs, AI review guide, linking a PR to a chat thread, label and reviewer pickers, multiple git hosts, GitHub Enterprise.
- Per-file "viewed" tracking, auto-merge, revert, branch updates.
- Filters beyond: state (open / closed / merged), involvement (mine / reviewing / all), draft.
- **Notifications.** No push or alerts for finished checks or new reviews in v1 (decided). Clients refresh on open, pull-to-refresh and after an action.
- **Forks.** Not supported: creating a PR from a fork into an upstream repo. The author does not work from forks (decided).

**Principle: minimal data.** Every response carries only the fields a screen draws. The list is a trimmed summary; the detail is fetched only when a PR is opened. No client re-derives anything the server can send (the usage and tool-call work in `docs/plan/orchestration.md` and the clients audit set this rule): status wording, check rollups and merge-readiness are computed once on the server.

---

## 1. What Already Exists (verified)

- **A stored GitHub token.** `internal/providers/github/credentials.go` keeps a PAT (0600 file, dir 0700), validated against `GET /user` on save (`pat.go`), with the login and scopes cached so status reads stay offline. `APIBase` is already a swappable var for tests.
- **Token injection into spawned commands.** `ghenv.go` puts `GH_TOKEN` in agent shells and `RedactSecrets` scrubs it from output. Raw `gh` already works for the agent; this plan adds *typed* tools, which go through approval gating that raw `gh` does not (it only inherits the run's exec approval).
- **A diff viewer on both clients.** Desktop `viewer/diff_viewer.rs` and Android `DiffView.kt` already render unified diffs. A PR's changed files reuse them.
- **The console kernel** (`internal/services/console/`) with the approval ceiling (`clampApproval`), which the agent methods plug into.

**Not present:** no GitHub REST/GraphQL client for pull requests, no PR routes, no PR proto, no PR UI.

---

## 2. Server

### 2.1 The GitHub service

New `internal/providers/github/pulls.go` (client) and `internal/services/pull_requests.go` (service). The service owns:
- **Repo resolution.** A project's PR repo is its `origin` remote, parsed to `owner/repo` (decided: `origin` only, no `base_repo` argument). Only `github.com` remotes resolve; anything else returns a clear "not a GitHub repository" error. A worktree shares its repo's remote, so it resolves the same way. To verify before building: how `GitService` exposes remotes.
- **Token loading at call time.** Read from the credential file per call; never cached in a package var, never returned to a client, never logged. All of this is the existing rule from the orchestration plan §Phase 7.
- **One HTTP client** with a timeout, `User-Agent`, and `Accept: application/vnd.github+json` / `X-GitHub-Api-Version` pinned.
- **Error mapping.** 401 → "token rejected, reconnect in Settings". 403 with `x-ratelimit-remaining: 0` → "rate limited until <time>". 403 otherwise → missing scope, naming the scope. 404 → not found *or* no access (GitHub deliberately returns 404 for private repos you cannot see; the message must say both). 422 → surface GitHub's own validation text.

### 2.2 Read path: GraphQL, one request

REST would need a request for the PR, one for combined status, one for check runs, one for reviews, and one for mergeability. **Use GraphQL for reads** so the list is one request and the detail is one request:

- **"Mine" and "Review requested" span every repo**, so they use GitHub `search` (`author:@me is:pr`, `review-requested:@me is:pr`), which is the most expensive list query. Mitigations: cache by query for 30s, request only the trimmed fields, page at 30, and surface a rate-limit message rather than retrying. A classic PAT needs `repo` (or a fine-grained token with access to the repos in question) for private repos to appear.
- **List** (`pullRequests`): per row only `number, title, state, isDraft, author.login, updatedAt, headRefName, baseRefName, reviewDecision, checks rollup state, additions, deletions, changedFiles`. Authored-by-me and review-requested are two `search` queries; the repo's open list is one `repository.pullRequests` query. No bodies, no files, no comments in the list.
- **Detail** (`pullRequest(number)`): the above plus `body`, `mergeable`, `mergeStateStatus`, merge methods the repo allows, `headRefOid` (needed for safe merge, below), the check runs (name, status, conclusion, url), the latest review per reviewer, and the last N issue comments (default 30, cursor for more). Files are **not** in the detail query; see 2.4.

**Mergeability is computed lazily by GitHub.** `mergeable` can come back `UNKNOWN` on the first read after a push while GitHub works it out. The server treats `UNKNOWN` as "checking", re-reads once after ~1s, and otherwise reports `unknown` rather than guessing. Clients show "checking…" and never treat unknown as mergeable.

### 2.3 Write path: REST

Mutations are simple and REST has the clearest error semantics:

| Action | Call | Notes |
| :--- | :--- | :--- |
| create | `POST /repos/{o}/{r}/pulls` | title, body, head, base, draft |
| update | `PATCH /repos/{o}/{r}/pulls/{n}` | title, body, base |
| close / reopen | `PATCH …/pulls/{n}` `state` | |
| ready for review | GraphQL `markPullRequestReadyForReview` | REST has no endpoint for this |
| draft | GraphQL `convertPullRequestToDraft` | |
| comment | `POST /repos/{o}/{r}/issues/{n}/comments` | top-level comment only in v1 |
| merge | `PUT …/pulls/{n}/merge` | see below |

**Merge safety.**
- Always send `sha` = the head SHA the user saw. If the branch moved since, GitHub returns 409 and the merge is refused instead of landing code nobody reviewed. The error tells the user to reload.
- `merge_method` must be one the repo allows (`merge` / `squash` / `rebase`); the detail response lists the allowed ones so a client never offers a method that will 405.
- Merging is never implied by create or update.
- A 405 (not mergeable: failing required checks, conflicts, missing review) surfaces GitHub's message verbatim.

### 2.4 Changed files

`GET /repos/{o}/{r}/pulls/{n}/files` returns per-file `filename, status, additions, deletions, patch`. It is paginated (max 3000 files) and `patch` is omitted by GitHub for very large or binary files. The server:
- returns **file list without patches** from the detail screen's "Files" tab first (path, status, +/−),
- fetches a single file's patch only when the user opens that file,
- converts to the existing unified-diff text so the **existing diff viewers render it unchanged**,
- reports "diff too large, view on GitHub" with the PR URL when GitHub omits the patch.

### 2.5 Routes and proto

`proto/console/v1/pull_requests.proto` (new), routes under `/api/pull-requests`:

```
GET  /api/pull-requests?scope=mine|review|repo&state=open|closed|merged&project_id=&draft=
     scope=mine    PRs I authored, across ALL my repos (GitHub search, not limited to registered projects)
     scope=review  PRs where my review is requested, across all my repos
     scope=repo    open PRs of one registered project (requires project_id)
GET  /api/pull-requests/:owner/:repo/:number            detail
GET  /api/pull-requests/:owner/:repo/:number/files      file list
GET  /api/pull-requests/:owner/:repo/:number/files/*path   one file's diff
POST /api/pull-requests                                  create
PATCH /api/pull-requests/:owner/:repo/:number            update / state / ready / draft
POST /api/pull-requests/:owner/:repo/:number/merge
POST /api/pull-requests/:owner/:repo/:number/comments
```

Messages (trimmed to what screens draw): `PullRequestSummary`, `PullRequestDetail`, `PullRequestCheck`, `PullRequestReview`, `PullRequestComment`, `PullRequestFile`, plus request messages. Enums stay plain strings with documented values (the existing proto convention). The server fills presentation fields once, in the same style as the usage and tool-call work:
- `checks_state` (`passing | failing | pending | none`), the rollup, computed from the individual runs,
- `merge_state` (`ready | blocked | conflicts | behind | checking | draft`) and a one-line `merge_blocker` reason ("2 checks failing", "review required"), so no client interprets GitHub's `mergeStateStatus`,
- `status_label` for the row badge.

Golden fixtures under `proto/testdata/pull_requests/` decoded by Go, desktop and Android tests, like every other domain.

### 2.6 Caching and refresh

- Short server cache (30s) keyed by (repo, query) with in-flight de-duplication, the same shape as `providers/usage/service.go`. A write invalidates that PR and the lists.
- ETag / `If-None-Match` on GraphQL is not available; for lists rely on the cache. Rate limit: 5000/hr for a PAT, and a GraphQL list costs about 1 point. Report remaining quota in an error, not on every response.
- No push stream in v1. Clients refresh on open, pull-to-refresh, and after any action. Live check updates are a follow-up (t3code streams them; not needed to read, check and merge).

---

## 3. Agent: `console.pr.*`

Typed methods in the console kernel, calling the **same service** as the routes. All writes are `TierWrite` and resolve through `permissions.Resolve` like every other write; the kernel is a transport, not a bypass.

```js
console.pr.list({ scope, state, project_id?, owner?, repo? })      // read
console.pr.read({ owner, repo, number })                           // read: checks, review state, mergeability, comments
console.pr.create({ owner?, repo?, head, base, title, body, draft? }) // write; draft defaults to TRUE
console.pr.update({ owner, repo, number, title?, body?, base? })   // write
console.pr.comment({ owner, repo, number, body })                  // write
console.pr.merge({ owner, repo, number, method, expected_head? })  // write, gated
console.pr.close / reopen / ready({ owner, repo, number })         // write
```

Rules:
- `owner`/`repo` default from the session's project (its `origin`) when omitted, so the common call is `console.pr.create({head, base, title, body})`.
- **Agent-created PRs are drafts by default** (decided). Pass `draft: false` to open one ready for review. A human flips a draft ready with `console.pr.ready` or the UI's Ready for review button. The desktop and Android create dialogs default the draft toggle on too.
- `merge` requires the head SHA from a prior `read` (`expected_head`); omitted, the server reads it and merges only that exact commit, then reports the SHA it merged. It is never the default path and never a side effect of another call.
- Results are minimal and carry the server-computed `merge_state` / `merge_blocker`, so the model is told *why* a merge cannot proceed rather than retrying.
- The token never appears in any result; `RedactSecrets` stays on the output path.
- Document the methods in the kernel `SKILL.md` (not in tool schemas, per the "cheap per turn" rule).

---

## 4. Desktop (Rust / GPUI)

New **Pull Requests** surface, reached from the sidebar. Reuse, do not rebuild:
- **List.** Rows: state icon, title, `#number`, author, relative time, checks badge, review badge, +/−. Segments: Mine / Review requested / Repo. State filter (Open default). Pull-to-refresh equivalent: a refresh button.
- **Detail** (tab-style, like the existing inspector pattern):
  - *Overview:* title, state, branch → base, description (existing markdown renderer), the `merge_state` banner with its one-line blocker, the checks list (name, status, link), reviewers with their latest verdict.
  - *Files:* file list with status and +/−; selecting one opens the existing diff viewer on the single-file patch.
  - *Comments:* the top-level timeline and a comment box.
- **Actions bar:** Merge (method picker limited to the allowed methods, disabled with the blocker as its tooltip when not `ready`), Close/Reopen, Ready for review, Open on GitHub. Merge asks for confirmation and shows the exact head SHA being merged.
- **Create:** a small dialog (title, body, base branch, draft) from a branch / session, prefilled from the session's branch.
- **Not connected:** an empty state that links to the GitHub connection in Settings, when there is no token or it lacks the needed scope.

Code placement follows the repo's pattern: DTOs re-exported from `console-proto` in `console-core/src/types/pull_requests.rs`, HTTP in `console-core/src/services/pull_requests.rs`, views in `console-ui/src/pull_requests/`. Tests in `tests/` folders, none inline.

---

## 5. Android (Kotlin / Compose)

Same capability as desktop, native to the platform; this is not a stripped-down client.
- **List screen** with the same three segments and a state filter, pull-to-refresh, trimmed rows.
- **Detail screen** with Overview / Files / Comments sections, the `merge_state` banner, checks, reviewers.
- **Merge, close/reopen, ready** from the detail screen, with a confirmation sheet showing the head SHA. Method picker offers only allowed methods.
- **Files** uses the existing `DiffView`; a file's patch loads on open.
- **Create PR** sheet from a session/branch.
- **Wire types** come from the proto through Wire like the other domains (no hand-written copies), and the shared fixtures are decoded by `apps/android/tests/*FixtureTest.kt`.

---

## 6. Security

- **Token scopes.** A classic PAT needs `repo` for private repos (`public_repo` for public only); a fine-grained token needs Pull requests read/write and Contents read, Metadata read. Fine-grained tokens return no `X-OAuth-Scopes`, so scope checks are advisory (existing note in `pat.go`). The service maps a 403/404 to a message naming what to grant instead of a generic failure.
- **Token handling.** Loaded per call, never cached, never returned, never logged, redacted from tool output.
- **Approval gating.** Every agent write is `TierWrite` through `permissions.Resolve`. The approval ceiling from the orchestration plan applies: a restricted run cannot merge by creating a more permissive session.
- **No merge by accident.** Merge always carries the reviewed head SHA, a confirmation in both UIs, and is never a side effect.
- **Untrusted text.** PR titles, bodies and comments are attacker-controlled (anyone can open a PR on a public repo). They reach the model only as data inside tool results, never into the system prompt, and the UIs render them through the existing markdown renderers (no raw HTML).

---

## 7. Phases

1. **Server service + read path.** Repo resolution, GraphQL list and detail, error mapping, cache, proto + fixtures, `GET` routes. *Verification:* `go test ./tests/api/ -run TestPullRequests -v` against an `httptest` GitHub (`APIBase` swap): list/detail mapping, `mine` / `review` scopes using search across repos, `UNKNOWN` mergeability retry, rate-limit and 404 messages, trimmed fields only.
2. **Server write path.** Create, update, state changes, ready/draft, comment, merge with head-SHA guard. *Verification:* same package: 409 on SHA mismatch is refused, 405 surfaces GitHub's message, disallowed merge method rejected, token never in any response body or log.
3. **Agent methods.** `console.pr.*` over the service, `TierWrite` gating, `SKILL.md`. *Verification:* `go test ./internal/services/console/ -run TestConsolePR -v`: defaults from project, merge requires/uses head SHA, result carries `merge_blocker`.
4. **Desktop.** List, detail, files, comments, actions, create, not-connected state. *Verification:* `cd apps/desktop && cargo test -p console-core pull_requests` for the DTO/fixture decode; the UI is checked by running the app against a real PR.
5. **Android.** Same surfaces. *Verification:* `cd apps/android && ./gradlew :app:assembleDebug` and `:app:testDebugUnitTest --tests "<PullRequestFixtureTest>"`.
6. **Follow-ups (not v1):** inline review comments and submitting reviews; live check updates; stacks; auto-merge.

Phases 1–3 are server-only and shippable on their own: the agent can create and merge PRs before any UI exists.

---

## 8. Decisions

All four earlier open questions are settled.

- **Repo resolution: `origin` only.** `console.pr.create` assumes the `origin` remote and takes no `base_repo`. The author does not work from forks; revisit only if that changes.
- **"Mine" means all of the user's repos**, not just registered projects. It is the search API (see 2.2), so it is cached and trimmed. "Review requested" works the same way. The repo-scoped list stays limited to registered projects.
- **Agent-created PRs default to draft.** Safer default; a human or `console.pr.ready` flips it. Both UIs default the toggle on.
- **No notifications in v1.** No push, polling or per-PR server state. This also means no live check updates; the feature is read, check and merge on demand.

Still open, not blocking v1:

- Whether `console.pr.merge` should refuse when the PR is a draft with a clear message, or just surface GitHub's own error. Recommendation: refuse early with the `merge_blocker` text.

---

## 9. Verification Matrix

| Test Case | Method | Expected Outcome |
| :--- | :--- | :--- |
| List / detail mapping | `go test ./tests/api/ -run TestPullRequests -v` | trimmed fields; checks rollup and `merge_state` computed server-side |
| Mergeability `UNKNOWN` | same | re-read once, then `checking`; never reported mergeable |
| Merge SHA guard | same | 409 on moved head is refused with a reload message |
| Merge method allowed | same | disallowed method rejected before the request |
| Error mapping | same | 401 / rate-limit / missing-scope / 404 produce distinct messages |
| Token safety | same | token absent from every response, error and log line |
| Agent defaults and gating | `go test ./internal/services/console/ -run TestConsolePR -v` | repo from project; writes are `TierWrite`; merge needs head SHA |
| Wire fixtures | Go, `cargo test -p console-proto`, Android fixture test | all three decode the same JSON |
| Desktop / Android | run against a real PR | list, detail, files, merge, close, create all work |
