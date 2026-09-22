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
- Mobile onboarding (`apps/mobile/index.tsx` `OnboardingScreen`) takes a
  backend URL via `useServerConnection`; post-connect auth lives in Account
  Settings (`apps/mobile/screens/settings/account-settings.tsx`) keyed off the
  provider catalog's `authMethod`.

## 2 UX Proposal
- Accounts is the primary home (desktop and mobile): a GitHub item showing
  `@username` + Connected/Not connected, with Connect / Re-login / Disconnect.
  Onboarding-time connect cannot serve existing installs (reinstalling would
  wipe local data), so the optional "Connect GitHub" onboarding card is a
  nice-to-have for fresh installs only — Accounts is the path that always works.
- v1 connection method: *personal access token*. Secure paste field in
  Accounts; server validates via `GET api.github.com/user` and stores it in
  `~/.console/github-creds.json`. Paste-once-and-done — no OAuth App to own,
  no browser round-trip, and it works for SSO/enterprise repos too.
- Device flow (*Connect with GitHub*: `POST /api/auth/github/device/start`,
  big `user_code`, "Approve on GitHub" button opening `verification_uri`,
  poll `device/status` until approved) is deferred polish for later — same
  storage and status row when it lands. It needs a shared GitHub OAuth App
  with a home account/org (see §8), which is exactly the setup PAT sidesteps.
- Scope is `repo` only. No repo browser, no GitHub file views in
  v1 — the deliverable is purely that the git CLI works everywhere. Cloning
  itself lives in the new-project dialog (see §5, last step), not here.

## 3 Server Design
- New `internal/providers/github/` package mirroring the claude/codex structure
  (`oauth.go` + `constants.go`): PAT accept/validate/store (validate via `GET
  api.github.com/user`), token stored at `~/.console/github-creds.json` via
  the existing `providers/shared` credential helpers, file mode 0600.
  Device-flow start/poll (`POST https://github.com/login/device/code`,
  `POST .../login/oauth/access_token` with `grant_type=device_code`) slots
  into the same package later.
- New `auth.AuthService` surface + routes under `/api/auth/github/*`
  (in `apps/server-go/internal/routes/auth.go`, `registerAuthRoutes`):
  `pat` (accept + validate + store a pasted fine-grained PAT), `status`
  (validate cached token against `api.github.com/user`, return `@username` +
  scopes, fold into `GetStatus`), `logout` (delete file). `device/start` +
  `device/status` (poll result, with expiry/denied mapping) arrive with device
  flow later.
- Credential injection via a **git credential-helper script**, not
  `GIT_ASKPASS` alone: a helper is consulted before any prompt in both TTY
  and non-TTY contexts, so it covers the PTY shells as well as agent-spawned
  git. The helper (small shell script shipped with the server, path resolved
  at runtime) reads the creds file and answers
  `username=x-access-token / password=<token>` for `github.com` hosts only.
- Wiring points:
  - `runGit` env (`apps/server-go/internal/services/git_service.go`, plus
    `BashJobManager.Start` env for agent-run shell commands): add `GIT_CONFIG_COUNT=1`,
    `GIT_CONFIG_KEY_0=credential.helper`,
    `GIT_CONFIG_VALUE_0=!<helper-path>` so agent git calls authenticate
    without touching global `~/.gitconfig`.
  - `PtyManager.Spawn` env (`apps/server-go/internal/services/pty_manager.go`): same three vars, so every interactive
    terminal inherits working git auth.
  - *(Optional — skip if you only use HTTPS URLs, no SSH setup needed)* `url."https://github.com/".insteadOf git@github.com:` via the same
    `GIT_CONFIG_*` channel so a pasted `git@github.com:org/repo` SSH-style URL also works over HTTPS with the same PAT. No SSH keys, `~/.ssh`, or `ssh-agent` needed — purely an HTTPS rewrite. Safe to omit.
- Boot behavior: helpers read the creds file at invocation time (not daemon
  start), so a VPS reboot or `console restart` keeps working with no re-login.
  Missing file → helper exits silently → git behaves exactly as today.
- Token hygiene: never log the token, never include it in clone URLs, never
  return it to clients (status returns username/scopes only), never pass it
  on the PTY command line. Refresh is N/A (device-flow user tokens don't
  expire unless revoked); `status` detects revocation and reports
  not-connected.

## 4 Client Changes (desktop + mobile)
- Accounts GitHub item (connect / re-login / disconnect) — existing installs
  connect here, never via reinstall. v1 uses the PAT screen; device-flow UI
  plugs into the same item later.
- `packages/api` `githubAuthService`: `getStatus`, `logout`, `submitPat`
  clients (`deviceStart`, `deviceStatus` arrive with device flow later).
- PAT screen with secure text entry, submitted once, never persisted
  client-side — the v1 connection path.
- Account Settings: GitHub row driven by extended auth status; disconnect with
  confirm; re-login reuses the PAT screen in v1.
- Onboarding card + `useGitHubDeviceLogin` hook (fresh installs only, code
  display + approve button + polling + Skip): deferred with device flow.

## 5 Implementation Steps
- 1: server provider module — PAT accept/validate/store, creds file
  read/write/validate/delete, username lookup. (Device start/poll exchange
  slots in later.)
- 2: `auth.AuthService` + `/api/auth/github/{pat,status,logout}` routes
  (`routes/auth.go`) + `GetStatus` extension,
  with unit tests for state/expiry/error mapping. (`device/*` routes arrive
  with device flow.)
- 3: credential-helper script + `GIT_CONFIG_*` wiring in `runGit`'s env and
  `PtyManager.Spawn`; verify both agent git and PTY git pick it
  up, and that missing-creds behaves as today.
- 4: SSH-URL rewrite via the same channel; test `git@github.com:org/repo`
  clone over HTTPS.
- 5: desktop + mobile Accounts GitHub item + PAT screen (v1); device-flow UI
  (`useGitHubDeviceLogin` + onboarding card) later.
- 6: docs: onboarding copy, token scope note, revocation/re-login path.
- 7 (last): new-project/clone dialog — see
  `docs/plan/new-project-dialog-plan.md`. Consumes the credential status and
  clone operation from this plan (steps 1–4); do it after they land.

## 6 Verification
- Fresh VPS: `install.sh` + `console start`, no keys on box. Accounts →
  paste PAT → `git clone <private-https-url>` succeeds in the
  mobile terminal tab AND via an agent run in the same session.
- Existing install with data: connect via Accounts (no reinstall) → private
  clone/push works.
- `git push` from the terminal tab works without any prompt.
- Pasted SSH remote (`git@github.com:org/private.git`) clones over HTTPS.
- Restart daemon / reboot box: git still works, no re-login.
- Revoke token on github.com → status shows not-connected; re-login recovers.
- Skip path (once the onboarding card exists): onboarding Skip → public clone works, private clone fails with
  stock git auth error (no crash, no leak in output).
- (Device-flow onboarding variant arrives with device flow later.)
- Confirm token appears nowhere in server logs, terminal scrollback, or
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
  the existing provider constants, e.g. `apps/server-go/internal/providers/claude/constants.go`)? Only matters when device flow is built (deferred) —
  the v1 PAT path needs no App. Device flow needs no client secret, but the App
  needs a home account/org. Alternative: bring-your-own client ID via env.
- Fine-grained PAT vs classic `repo` scope for the paste option — recommend
  fine-grained with repository access, validate `X-OAuth-Scopes`/permissions
  on submit.
