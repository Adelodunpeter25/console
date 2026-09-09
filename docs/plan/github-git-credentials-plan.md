# GitHub Git-Credentials Plan

Goal: on a brand-new machine (e.g. fresh Hetzner VPS with `console start` only),
`git clone/push/pull` against private GitHub repos works both for the server
(agent `GitService` calls) and for the user typing in the mobile terminal tab —
with zero SSH keys or `gh auth` setup on the box. One optional, skippable
"Connect GitHub" step during mobile onboarding provisions it.

## 1 Current State
- Server `GitService` (`apps/server/api/src/services/git.service.ts`) runs git
  via `execShell` (`apps/server/api/src/utils/exec.ts`), which inherits the
  server process env. No credential configuration of any kind today.
- Interactive shells spawn in `TerminalPtyManager.startShell`
  (`apps/server/api/src/terminal/pty.manager.ts`) with `{...process.env,
  TERM, CONSOLE_TERMINAL}` — they inherit whatever the daemon had at boot.
- Both paths therefore pick up one shared mechanism for free if the server
  process (and its children) is taught the GitHub token.
- Provider OAuth precedent exists: `AuthService.getLoginUrl/handleCallback`
  (`apps/server/api/src/services/auth.service.ts`), token files under
  `~/.console/<type>-creds.json` via `token-store.ts` (0600-style, server-side
  only, never sent to clients), per-provider config via `provider-config.ts`.
- Mobile onboarding (`apps/mobile/index.tsx` `OnboardingScreen`) takes a
  backend URL via `useServerConnection`; post-connect auth lives in Account
  Settings (`apps/mobile/screens/settings/account-settings.tsx`) keyed off the
  provider catalog's `authMethod`.

## 2 UX Proposal
- Onboarding: after a successful backend-URL connect + test, offer an optional
  "Connect GitHub" card (skippable — public repos keep working without it).
  Tapping it calls `POST /api/auth/github/device/start`, shows the returned
  `user_code` big with a "Approve on GitHub" button opening
  `verification_uri` in the system browser. Poll `device/status` until the
  user approves; success shows `@username` and continues to the app.
- Settings → Accounts: new GitHub row showing `@username` + Connected/Not
  connected, with Connect / Re-login / Disconnect. Same device-flow UI.
- Fallback: "Use a personal access token instead" link opening a secure paste
  field; server validates via `GET api.github.com/user` and stores the same
  way (covers enterprise/SSO users who can't use the shared OAuth App).
- Scope is `repo` only. No repo browser, no clone UI, no GitHub file views in
  v1 — the deliverable is purely that the git CLI works everywhere.

## 3 Server Design
- New `github.ts` provider module mirroring the antigravity/codex structure:
  device-flow start (`POST https://github.com/login/device/code`), poll
  (`POST .../login/oauth/access_token` with `grant_type=device_code`), token
  stored at `~/.console/github-creds.json` via the existing token-store
  pattern, file mode 0600.
- New `AuthService` surface + routes under `/api/auth/github/*`:
  `device/start`, `device/status` (poll result, with expiry/denied mapping),
  `status` (validate cached token against `api.github.com/user`, return
  `@username` + scopes, fold into `getAuthStatus`), `logout` (delete file),
  `pat` (accept + validate + store a pasted fine-grained PAT).
- Credential injection via a **git credential-helper script**, not
  `GIT_ASKPASS` alone: a helper is consulted before any prompt in both TTY
  and non-TTY contexts, so it covers the PTY shells as well as agent-spawned
  git. The helper (small shell script shipped with the server, path resolved
  at runtime) reads the creds file and answers
  `username=x-access-token / password=<token>` for `github.com` hosts only.
- Wiring points:
  - `execShell`/`spawnCapture` env: add `GIT_CONFIG_COUNT=1`,
    `GIT_CONFIG_KEY_0=credential.helper`,
    `GIT_CONFIG_VALUE_0=!<helper-path>` so agent git calls authenticate
    without touching global `~/.gitconfig`.
  - `pty.manager.ts startShell` env: same three vars, so every interactive
    terminal inherits working git auth.
  - Add `url."https://github.com/".insteadOf git@github.com:` via the same
    `GIT_CONFIG_*` channel so pasted SSH remote URLs work over HTTPS too.
- Boot behavior: helpers read the creds file at invocation time (not daemon
  start), so a VPS reboot or `console restart` keeps working with no re-login.
  Missing file → helper exits silently → git behaves exactly as today.
- Token hygiene: never log the token, never include it in clone URLs, never
  return it to clients (status returns username/scopes only), never pass it
  on the PTY command line. Refresh is N/A (device-flow user tokens don't
  expire unless revoked); `status` detects revocation and reports
  not-connected.

## 4 Mobile Changes
- `packages/api` `githubAuthService`: `deviceStart`, `deviceStatus`,
  `getStatus`, `logout`, `submitPat` clients.
- Onboarding: post-connect GitHub card in `OnboardingScreen` with code display
  + approve button + polling + Skip. New `useGitHubDeviceLogin` hook owning
  the poll lifecycle (interval + expiry timeout + cancel on unmount).
- Account Settings: GitHub row driven by extended auth status; reuse the same
  hook for connect/re-login; disconnect with confirm.
- PAT fallback screen with secure text entry, submitted once, never persisted
  client-side.

## 5 Implementation Steps
- 1: server provider module — device start/poll exchange, creds file
  read/write/validate/delete, username lookup.
- 2: `AuthService` + `/api/auth/github/*` routes + `getAuthStatus` extension,
  with unit tests for state/expiry/error mapping.
- 3: credential-helper script + `GIT_CONFIG_*` wiring in `exec.ts` env path
  and `pty.manager.ts startShell`; verify both agent git and PTY git pick it
  up, and that missing-creds behaves as today.
- 4: SSH-URL rewrite via the same channel; test `git@github.com:org/repo`
  clone over HTTPS.
- 5: mobile API client + `useGitHubDeviceLogin` + onboarding card + Account
  Settings row + PAT fallback.
- 6: docs: onboarding copy, token scope note, revocation/re-login path.

## 6 Verification
- Fresh VPS: `install.sh` + `console start`, no keys on box. Onboarding →
  Connect GitHub → approve → `git clone <private-https-url>` succeeds in the
  mobile terminal tab AND via an agent run in the same session.
- `git push` from the terminal tab works without any prompt.
- Pasted SSH remote (`git@github.com:org/private.git`) clones over HTTPS.
- Restart daemon / reboot box: git still works, no re-login.
- Revoke token on github.com → status shows not-connected; re-login recovers.
- Skip path: onboarding Skip → public clone works, private clone fails with
  stock git auth error (no crash, no leak in output).
- PAT path: paste fine-grained PAT (repo scope) → same clone/push success.
- Confirm token appears nowhere in server logs, terminal scrollback, or
  network responses to the client.

## 7 Out of Scope
- Server API authentication / pairing tokens, CORS tightening, bind address —
  tracked separately; this plan assumes a trusted network path to the VPS.
- Any GitHub repo browsing, cloning UI, or commit/push buttons on mobile.
- GHE (GitHub Enterprise Server) hosts in v1 — helper answers `github.com`
  only; extend by storing per-host entries later.
- SSH agent forwarding or deploy keys — explicitly not the mechanism.
- Non-GitHub hosts (GitLab, Bitbucket) — same helper pattern applies later.

## 8 Open Questions
- Who owns the shared GitHub OAuth App (client ID baked into the server like
  the Antigravity constants)? Device flow needs no client secret, but the App
  needs a home account/org. Alternative: bring-your-own client ID via env.
- Fine-grained PAT vs classic `repo` scope for the paste fallback — recommend
  fine-grained with repository access, validate `X-OAuth-Scopes`/permissions
  on submit.
