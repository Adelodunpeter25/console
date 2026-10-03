# GitHub Git-Credentials Plan

Goal: on a brand-new machine (e.g. fresh Hetzner VPS with `console start` only),
`git clone/push/pull` against private GitHub repos works both for the server
(agent `GitService` calls) and for the user typing in the mobile terminal tab —
with zero SSH keys or `gh auth` setup on the box. One optional, skippable
"Connect GitHub" step during mobile onboarding provisions it.

## 1 Current State
- Server `GitService` (`apps/server-go/internal/services/git_service.go`) runs git
  via the `runGit` helper (`os/exec`, which inherits the
  server process env). No credential configuration of any kind today.
- Interactive shells spawn in `PtyManager.Spawn`
  (`apps/server-go/internal/services/pty_manager.go`) with the cached process
  env plus `TERM`, `CONSOLE_TERMINAL`, `CONSOLE_TERMINAL_ID` — they inherit whatever the daemon had at boot.
- Both paths therefore pick up one shared mechanism for free if the server
  process (and its children) is taught the GitHub token.
- Provider OAuth precedent exists: `auth.AuthService.GetLoginURLFor/HandleCallbackFor`
  (`apps/server-go/internal/auth/auth.go`), token files under
  `~/.console/<type>-creds.json` via the `providers/shared` credential helpers
  (`CredentialPath`/`SaveCredentialFile`, dir 0700/file 0600, server-side
  only, never sent to clients), per-provider config via each provider's
  `constants.go` (e.g. `apps/server-go/internal/providers/claude/constants.go`).
- Clients are native and re-implement the API by hand (neither consumes
  `packages/api`): desktop is Rust/GPUI (`apps/desktop`, Accounts page at
  `crates/console-ui/src/settings/accounts_page.rs` driven by the provider
  catalog + auth status, HTTP via `crates/console-core/src/services/`), and
  Android is Kotlin/Compose (`apps/android`, onboarding under
  `feature/onboarding`, account settings under `feature/settings`, network via
  `data/api` + `data/repo`). Shared auth-status DTOs are mirrored by hand on
  both sides, so any new server status field needs a matching client type.

## 2 UX Proposal
- Accounts is the primary home (desktop and mobile): a GitHub item showing
  `@username` + Connected/Not connected, with Connect / Re-login / Disconnect.
  Onboarding-time connect cannot serve existing installs (reinstalling would
  wipe local data), so the optional "Connect GitHub" onboarding card is a
  nice-to-have for fresh installs only — Accounts is the path that always works.
- v1 connection method: *personal access token*. Secure paste field in
  Accounts; server validates via `GET api.github.com/user` and stores it in
  `<console storage>/github-creds.json`. Paste-once-and-done — no OAuth App to
  own, no browser round-trip, and it works for SSO/enterprise repos too.
- Device flow (*Connect with GitHub*: `POST /api/auth/github/device/start`,
  big `user_code`, "Approve on GitHub" button opening `verification_uri`,
  poll `device/status` until approved) is deferred polish for later — same
  storage and status row when it lands. It needs a shared GitHub OAuth App
  with a home account/org (see §8), which is exactly the setup PAT sidesteps.
- Scope is `repo` only. No repo browser, no GitHub file views in
  v1 — the deliverable is purely that the git CLI works everywhere. Cloning
  itself lives in the new-project dialog (see §5, last step), not here.

## 3 Server Design

> **Status: steps 1–4 implemented.** Notes below marked *(as built)* record
> decisions that differ from the original draft, and the traps worth knowing
> before extending this.

- New `internal/providers/github/` package: `credentials.go` (file
  read/write/clear, cached `username` + `scopes`), `pat.go` (validate via
  `GET api.github.com/user`), `helper.go` (the git credential-helper
  protocol). Device-flow start/poll slots into the same package later.
- New `auth.AuthService` surface + routes under `/api/auth/github/*`
  (`apps/server-go/internal/routes/auth.go`, `RegisterAuthRoutes` — now
  exported so route tests can build an app, mirroring `RegisterMCPRoutes`):
  `POST /github/pat` (accept + validate + store), `GET /github/status`,
  `POST /github/logout`. `AuthStatus` gains a `github` row.
- Credential injection via a **git credential helper**, not `GIT_ASKPASS`:
  a helper is consulted before any prompt in both TTY and non-TTY contexts,
  so it covers PTY shells as well as agent-spawned git.
- *(as built)* **The helper is a mode of the existing `console` binary, not a
  shipped shell script.** `git` runs the helper path as a subprocess, and a
  shell script would have to parse `github-creds.json` — needing `jq`, which a
  fresh VPS may not have. `cmd/console/main.go` dispatches on
  `CONSOLE_GIT_CREDENTIAL_HELPER=1` (set inline by the config value) and calls
  `github.RunCredentialHelper`.
  **That check must stay first in `main.go`.** The daemon runs under
  `CONSOLE_SERVE=1` and git inherits it, so checking serve mode first would
  boot a second server instead of answering. A hidden cobra subcommand is *not*
  sufficient on its own — cobra is only reached after the serve branch.
  `console git-credential-helper get` still exists for manual debugging.
- *(as built)* **Injection happens once, into the server's own environment**
  (`internal/gitconfig.Configure`, called first in `serve.Run`), not per call
  site. `runGit`, the agent bash tool (both sync and background, via
  `tools.mergedEnv`), and `PtyManager.Spawn` all already inherit
  `os.Environ()`, so this covers them plus any future call site.
  **It must run before the first PTY spawn**: `PtyManager.baseEnv()` caches
  `os.Environ()` in a `sync.Once`, so a later `Setenv` silently misses every
  terminal.
- `GIT_CONFIG_COUNT=3`: `credential.helper` = `!CONSOLE_GIT_CREDENTIAL_HELPER=1 <exe>`,
  plus `url."https://github.com/".insteadOf` for both `git@github.com:` and
  `ssh://git@github.com/`. The `insteadOf` rewrite is **not optional** — agents
  emit SSH remotes by default, which would otherwise fail
  `Permission denied (publickey)` and defeat the point. Nothing global is
  written.
- *(as built)* **Auth status is local-only.** `username`/`scopes` are cached in
  the credential file at accept time, so `GET /api/auth/status` makes no
  network call and cannot stall the Claude/Codex/Antigravity rows. Consequence:
  a revoked token still reports connected until re-login or disconnect; the
  symptom is a git auth failure, not a status change.
- *(as built)* **Scopes are advisory.** Classic PATs return `X-OAuth-Scopes`;
  fine-grained PATs return no such header, so an empty list is normal and never
  a validation failure. A `403` is reported as rate-limiting, never as a bad
  token — telling the user to re-paste a working token would not help.
- *(as built)* Credential path resolves through `utils.GitHubCredentialsPath()`
  (`CONSOLE_STORAGE_DIR` / `CONSOLE_ENV` aware), not the providers'
  `shared.CredentialPath` which pins `~/.console`. `SaveCredential` re-asserts
  the directory mode because `~/.console` is created by other subsystems under
  the process umask (typically 0755).
- Helper answers `username=x-access-token / password=<token>` for `github.com`
  **only**, matched exactly (no suffix match, so `github.com.evil.test` gets
  nothing), and only for `https`. `store`/`erase` are acknowledged no-ops so a
  git command in a terminal cannot overwrite or drop the token.
- Boot behavior: the helper reads the creds file at invocation time, so a VPS
  reboot or `console restart` keeps working with no re-login. Missing file →
  helper stays silent → git behaves exactly as today.
- Token hygiene: never logged, never in clone URLs, never returned to clients
  (responses carry username/scopes only), never on a command line. The token
  file is 0600.

## 4 Client Changes (desktop + Android)
- Accounts GitHub item (connect / re-login / disconnect) — existing installs
  connect here, never via reinstall. v1 uses the PAT screen; device-flow UI
  plugs into the same item later.
- *(as built)* **The row must be hardcoded, not catalog-driven.** Both
  Accounts screens render `providers.filter(authMethod != "none")` from
  `ListProviders()` (`providers/catalog.go`), so a `github` catalog entry would
  leak into every model picker via `FindModel`. Render it as a sibling row
  below the provider list, with its own status type
  (`{connected, username, scopes}`) rather than the shared
  `ProviderAuthStatus`, which only carries `loggedIn` + `email`.
- Desktop (`apps/desktop`, Rust/GPUI): GitHub row on `AccountsPage`
  (`crates/console-ui/src/settings/accounts_page.rs`); new `console-core`
  service for `/api/auth/github/{pat,status,logout}` alongside the existing
  auth service. Add the `github` field to the hand-mirrored
  `AuthStatusResponse` in `crates/console-core/src/types/auth.rs`.
- Android (`apps/android`, native Kotlin/Compose): GitHub row in
  `feature/settings/AccountSettings.kt` + PAT screen with secure text entry
  through the existing `data/api` client; submitted once, never persisted
  client-side. Mirror the field in `data/model/AuthShim.kt`.
- Onboarding card (fresh installs only, code display + approve button +
  polling + Skip): deferred with device flow.

## 5 Implementation Steps
- 1 ✅ server provider module — `internal/providers/github/{credentials,pat,helper}.go`
  (PAT accept/validate/store, file read/write/clear, username lookup).
  Tests: `tests/providers/github_test.go`.
- 2 ✅ `auth.AuthService` + `/api/auth/github/{pat,status,logout}` routes +
  offline `GetStatus` extension. Tests: `tests/api/github_auth_test.go`.
- 3 ✅ credential-helper mode in `cmd/console` + `internal/gitconfig`
  injection at the top of `serve.Run`.
- 4 ✅ SSH→HTTPS `insteadOf` rewrite on the same channel.
  Tests: `tests/services/gitconfig_test.go` — drives a real `git` and a real
  purpose-built `console` binary, including the `CONSOLE_SERVE=1` ordering case.
- 5 ⬜ desktop `AccountsPage` + Android settings GitHub item + PAT screen (v1);
  device-flow UI (status polling + onboarding card) later.
- 6 ⬜ docs: onboarding copy, token scope note, revocation/re-login path.
- 7 ⬜ (last) new-project/clone dialog — see
  `docs/plan/new-project-dialog-plan.md`. Consumes the credential status and
  clone operation from this plan (steps 1–4); do it after they land.

### Server test commands
```
cd apps/server-go
go test ./tests/providers/ -run GitHub -v
go test ./tests/api/      -run GitHub -v
go test ./tests/services/ -run GitConfig -v
```

## 6 Verification

Automated (server, steps 1–4):
- `tests/providers/github_test.go` — file mode 0600 / dir 0700, path override,
  idempotent clear, PAT success + error mapping (401 / 403 / 5xx / no-login /
  unreachable / blank), fine-grained PAT with no scope header, helper host
  matching incl. `github.com.evil.test`, protocol handling, silence without a
  token, `store`/`erase` no-ops.
- `tests/api/github_auth_test.go` — PAT stores + reports username, invalid
  token 400 and not stored, unreachable GitHub 502, a failed attempt keeps the
  working token, status, logout (idempotent), `GetStatus` includes `github`
  and never serializes the token.
- `tests/services/gitconfig_test.go` — the three `GIT_CONFIG_*` triples, real
  `git` resolving the helper, `insteadOf` rewriting `git@` and `ssh://` forms
  while leaving HTTPS and non-GitHub remotes alone, and end-to-end
  `git credential fill` against a real `console` binary — including under
  `CONSOLE_SERVE=1` to prove the helper wins over serve mode.

Manual, after step 5 lands:
- Fresh VPS: `install.sh` + `console start`, no keys on box. Accounts →
  paste PAT → `git clone <private-https-url>` succeeds in the
  mobile terminal tab AND via an agent run in the same session.
- Existing install with data: connect via Accounts (no reinstall) → private
  clone/push works.
- `git push` from the terminal tab works without any prompt.
- Pasted SSH remote (`git@github.com:org/private.git`) clones over HTTPS.
- Restart daemon / reboot box: git still works, no re-login.
- Revoke token on github.com → git operations fail auth → re-login recovers.
  *(Status stays "connected" until then by design — status is local-only.)*
- Skip path (once the onboarding card exists): onboarding Skip → public clone
  works, private clone fails with stock git auth error (no crash, no leak).
- Confirm the token appears nowhere in server logs, terminal scrollback, or
  network responses to the client.

## 7 Out of Scope
- Server API authentication / pairing tokens, CORS tightening, bind address —
  tracked separately; this plan assumes a trusted network path to the VPS.
- Any GitHub repo browsing, cloning UI, or commit/push buttons on mobile.
  (Clone UI lives in the desktop new-project dialog —
  `docs/plan/new-project-dialog-plan.md`.)
- Creating new GitHub repos from console (the dialog clones or starts blank
  locally only).
- GHE (GitHub Enterprise Server) hosts in v1 — helper answers `github.com`
  only; extend by storing per-host entries later.
- SSH agent forwarding or deploy keys — explicitly not the mechanism.
- Non-GitHub hosts (GitLab, Bitbucket) — same helper pattern applies later.

## 8 Open Questions
- Who owns the shared GitHub OAuth App (client ID baked into the server like
  the existing provider constants, e.g.
  `apps/server-go/internal/providers/claude/constants.go`)? Only matters when
  device flow is built (deferred) — the v1 PAT path needs no App. Device flow
  needs no client secret, but the App needs a home account/org. Alternative:
  bring-your-own client ID via env.
- Fine-grained PAT vs classic `repo` scope for the paste option — recommend
  fine-grained with repository access. *(Resolved in implementation: scopes are
  stored and displayed but never used to accept or reject a token, because
  fine-grained PATs report no scopes at all. Validation is "does this token
  authenticate", nothing more.)*
- Device flow needs a way to revoke: `POST /github/logout` deletes the local
  file only, so a device-flow token would also want a GitHub-side revocation
  call.
