# Cloudflare account control plane

Status: proposed plan. No implementation yet.

## 0. Decisions already made

- Console will host a central account service on Cloudflare.
- The service is a Cloudflare Worker plus Durable Objects.
- Account identity is email address plus password, with a required display name. No GitHub OAuth in v1.
- Device apps sign into the account once and discover all servers from it.
- Servers are added with an explicit "console enroll" command.
- Two enrollment modes are in v1:
  - browser approval link: console enroll prints a link and the CLI polls
  - headless one-time code: create a code in the dashboard, run console enroll --code CODE
- Existing device pairing is not preserved as the primary path. Re-pairing is acceptable.
- A local fallback token is included so a machine can be used from localhost if the account service is unreachable.
- Account-specific settings and future features live in the per-account Durable Object.
- User-level skills under `~/.agents/skills` are synced by account in v1. Project skills stay project-local.
- Local skill changes are uploaded automatically; account changes are mirrored automatically to enrolled servers.
- The control plane is the source of truth for account skills; enrolled servers mirror them under `~/.console/sync/skills`.

## 1. Goals

Remove repeated pairing. One account owns many servers and many devices. Signing in on a new desktop or phone should list all servers on the account without entering each server host and pairing code.

Keep user data off the control plane. The Worker only stores account identity, server registry, device registry, enrollment state, settings, account skills, and token metadata. Skills are prompt/workflow text, not secret storage; users must not put credentials in skills. Source code, chats, files, provider credentials, terminal sessions, and API keys stay on the user's servers.

Give servers a way to verify app tokens without calling Cloudflare on every request.

## 2. Non-goals for v1

- No GitHub, Google, Apple, passkey, or other OAuth/social login.
- No email provider, email verification, password reset, recovery codes, or security notices in v1. If the password is forgotten, the account cannot be recovered through the product.
- No organizations, teams, shared servers, roles, or invite flows. One account owns its own servers.
- No billing, subscriptions, or entitlements, but the data model must leave space for them.
- No end-to-end encrypted settings sync. Settings are account data stored by Console.
- No end-to-end encrypted skill sync. Skill bodies are account data stored by Console; they are not a secret store.
- No migration of existing paired devices. Old pairing can be removed or kept only as a hidden local escape path, not as the main flow.
- No change to how agent runs, files, projects, or provider credentials are stored on the server.

## 3. Architecture

There are two planes.

Control plane: Cloudflare Worker hosted by Console.
- Owns accounts and passwords.
- Owns the server and device registry.
- Runs enrollment approval.
- Issues short-lived, server-scoped access tokens.
- Serves the public signing keys used by servers.

Data plane: the existing Go server and its clients.
- Apps connect directly to a server over iroh or HTTPS, as they do today.
- Each request to a server carries a short-lived account token.
- The server verifies that token locally using the account public key.
- No chat or file data passes through Cloudflare.

Flow:

    email + password
          |
    Cloudflare account service
    /account  /servers  /token  /enroll
          |
          | issues signed token
          v
    desktop / phone app  ---- direct iroh/HTTPS ---->  user server

## 4. New repo: apps/control-plane

Add a new Cloudflare Worker project at apps/control-plane.

Suggested layout:

    apps/control-plane/
      src/
        index.ts
        env.ts
        errors.ts
        durable/
          account.ts
          enrollment.ts
          token_keys.ts
        auth/
          password.ts
          session.ts
          app_token.ts
          cookies.ts
        routes/
          account.ts
          auth.ts
          enrollment.ts
          servers.ts
          devices.ts
          settings.ts
          skills.ts
          keys.ts
          internal.ts
        validation/
          schemas.ts
      tests/
        auth.test.ts
        enrollment.test.ts
        tokens.test.ts
        account_do.test.ts
        skills.test.ts
      wrangler.toml
      package.json
      tsconfig.json
      README.md

Use Cloudflare Vitest-style worker tests where possible. Keep all request parsing and response shapes small and explicit. Prefer a tiny router over a large framework.

## 5. Cloudflare bindings and secrets

wrangler.toml bindings:

- ACCOUNT_DO: Durable Object namespace, SQLite storage backend.
- ENROLLMENT_DO: Durable Object namespace, SQLite storage backend.
- TOKEN_KEYS_DO: Durable Object namespace for signing key metadata and rotation.
- CONTROL_PLANE_DB not needed initially; account state belongs in Account DOs.

Secrets:

- SESSION_SECRET: secret used to sign or encrypt browser session cookies if cookies are not opaque random tokens stored in the DO.
- TOKEN_SIGNING_PRIVATE_KEY: private key for signing access tokens. Use ES256 / ECDSA P-256 via WebCrypto. Generate offline and manage as a Worker secret.
- CONTROL_PLANE_URL: public URL of the Worker, e.g. https://api.console.example.

Notes:

- ES256 is preferred over custom Ed25519 assumptions because WebCrypto support for ECDSA is broad.
- The public half of the signing key is served at GET /.well-known/console-keys and cached by servers.
- Key ids are included in issued tokens so rotation is possible without invalidating every token instantly.

## 6. Durable Object model

### 6.1 Account DO

ID: account id, generated with crypto.randomUUID() at signup.

Tables:

    accounts (
      id TEXT PRIMARY KEY,
      email TEXT NOT NULL UNIQUE,
      password_hash TEXT NOT NULL,
      display_name TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      updated_at INTEGER NOT NULL,
      token_version INTEGER NOT NULL DEFAULT 1
    )

    auth_sessions (
      id TEXT PRIMARY KEY,
      account_id TEXT NOT NULL,
      user_agent TEXT NOT NULL DEFAULT '',
      created_at INTEGER NOT NULL,
      expires_at INTEGER NOT NULL,
      revoked_at INTEGER
    )

    app_sessions (
      id TEXT PRIMARY KEY,
      account_id TEXT NOT NULL,
      device_id TEXT NOT NULL,
      refresh_token_hash TEXT NOT NULL UNIQUE,
      name TEXT NOT NULL,
      platform TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      last_used_at INTEGER NOT NULL,
      expires_at INTEGER NOT NULL,
      revoked_at INTEGER
    )

    devices (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      platform TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      last_seen_at INTEGER NOT NULL,
      revoked_at INTEGER
    )

    servers (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      server_secret_hash TEXT NOT NULL,
      node_id TEXT NOT NULL DEFAULT '',
      public_url TEXT NOT NULL DEFAULT '',
      status TEXT NOT NULL DEFAULT 'active',
      created_at INTEGER NOT NULL,
      last_heartbeat_at INTEGER NOT NULL,
      revoked_at INTEGER
    )

    server_events (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      server_id TEXT NOT NULL,
      kind TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      detail TEXT NOT NULL DEFAULT ''
    )

    settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL,
      updated_at INTEGER NOT NULL
    )

    skills (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      name_key TEXT NOT NULL,
      description TEXT NOT NULL DEFAULT '',
      content TEXT NOT NULL,
      version INTEGER NOT NULL DEFAULT 1,
      created_at INTEGER NOT NULL,
      updated_at INTEGER NOT NULL,
      deleted_at INTEGER
    )

    sync_state (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL,
      updated_at INTEGER NOT NULL
    )

    server_sync_state (
      server_id TEXT PRIMARY KEY,
      skills_revision INTEGER NOT NULL DEFAULT 0,
      last_pushed_at INTEGER,
      last_pulled_at INTEGER
    )

Rules:

- password_hash uses PBKDF2-SHA256 via Cloudflare WebCrypto with a high iteration count and a 16-byte random salt.
- Plain passwords are never logged, returned, or stored.
- Refresh tokens and server secrets are stored hashed. Only the first response after enrollment/redeem contains the plain secret.
- Email is normalized to lowercase and trimmed.
- The unique index on email is enforced in the DO transaction.
- token_version is the account-wide revocation knob. Bumping it invalidates all account tokens after servers receive the new expected version.
- Skill names are normalized to a lowercase name_key. At most one active skill may use a name_key; deleted rows keep history.
- Account skills use optimistic version checks. A write with a stale version returns a conflict instead of overwriting another device.
- Every skill create, update, or delete increments the account skills_revision stored in sync_state.

### 6.2 Enrollment DO

ID: enrollment request id or enrollment code, depending on route.

Tables:

    enrollment_requests (
      id TEXT PRIMARY KEY,
      account_id TEXT,
      requested_name TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      expires_at INTEGER NOT NULL,
      approved_at INTEGER,
      consumed_at INTEGER,
      revoked_at INTEGER,
      server_id TEXT
    )

    enrollment_codes (
      code_hash TEXT PRIMARY KEY,
      account_id TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      expires_at INTEGER NOT NULL,
      consumed_at INTEGER,
      revoked_at INTEGER,
      requested_name TEXT NOT NULL DEFAULT ''
    )

Rules:

- Request ids are 8 hex characters, like the existing server id style, or 12 human-safe characters. They are not secrets.
- Enrollment codes are 16 to 20 characters from a human-safe alphabet, grouped for readability, e.g. K7MT-2Q9X-4BWN-R8QP.
- Codes and request ids expire in 10 minutes by default.
- A code is single use. Approval must atomically mark it consumed.
- The Enrollment DO delegates the actual server creation to the Account DO.

### 6.3 Token keys DO

Tables:

    keys (
      kid TEXT PRIMARY KEY,
      public_jwk TEXT NOT NULL,
      algorithm TEXT NOT NULL DEFAULT 'ES256',
      created_at INTEGER NOT NULL,
      retired_at INTEGER
    )

The private key is a Worker secret. The DO only tracks public metadata and rotation state. Token verification uses the public JWK served by the keys route and cached by servers.

## 7. HTTP surface

All Worker responses use JSON. Errors use a stable shape:

    { "error": { "code": "invalid_credentials", "message": "Email or password is incorrect." } }

### 7.1 Browser/auth routes

Used by the dashboard web app.

- POST /auth/signup
  - body: email, password, display_name (required)
  - creates account
  - returns account id and a session cookie
- POST /auth/login
  - body: email, password
  - sets an httpOnly, secure, sameSite cookie
- POST /auth/logout
  - revokes current browser session
- GET /auth/me
  - returns account profile, including display name and email


### 7.2 Dashboard account routes

- GET /account
  - returns profile, servers, devices, settings summary
- PATCH /account
  - update display name
- GET /account/settings
- PUT /account/settings
  - key/value settings, size-limited
- GET /account/skills
  - returns skill metadata without full bodies
- POST /account/skills
  - body: name, description?, content
- GET /account/skills/:id
  - returns one full skill body
- PUT /account/skills/:id
  - body: name?, description?, content?, expected_version
  - stale version returns a conflict
- DELETE /account/skills/:id
  - soft-deletes the skill and increments skills_revision
- POST /account/revoke-all-devices
  - revokes app sessions and bumps token_version
- POST /account/tokens/refresh
  - not browser-facing unless the dashboard needs it; apps use their own refresh token

### 7.3 Dashboard enrollment routes

- POST /enroll-codes
  - creates a headless enrollment code for the signed-in account
  - returns the plain code once plus expiry
- GET /enrollments/:id
  - returns request state for the approval page
- POST /enrollments/:id/approve
  - approves a browser enrollment request
  - returns server id and server secret once to the browser
- POST /enrollments/:id/reject
- GET /servers
- GET /servers/:id
- POST /servers/:id/rename
- POST /servers/:id/revoke
  - status becomes revoked and token_version is bumped
- GET /devices
- POST /devices/:id/revoke

### 7.4 Machine/server routes

These are called by the console CLI and server binary. They do not use browser cookies.

- POST /v1/enroll/start
  - body: requested_name, platform, version
  - creates an enrollment request
  - returns request id, approval URL, expires_at, poll_interval
- GET /v1/enroll/status?request=ID
  - returns pending | approved | rejected | expired
  - on approved, returns account label, server id, server secret, public keys URL, and control plane URL
  - the server secret is returned only while the request is approved and un-consumed; after consumption the CLI cannot get it again
- POST /v1/enroll/code
  - body: code, requested_name, platform, version
  - redeems a dashboard-created code
  - returns the same enrollment result as the approved browser flow
- POST /v1/servers/heartbeat
  - authenticated with server id plus server secret
  - body: name?, node_id?, public_url?, version, token_version, synced_skills_revision?
  - updates last heartbeat, connection info, and the last skills revision the server confirmed
  - returns current token_version, revoked state, skills_revision, skills_changed, and any control-plane config the server should know
- GET /v1/sync/skills
  - authenticated with server id plus server secret
  - returns { revision, skills: [...] } for all active account skills
  - full skill bodies are returned here; list routes for apps can return metadata only

### 7.5 App/device routes

Used by desktop and Android after email/password sign-in.

- POST /v1/app/login
  - body: email, password, device_name, platform
  - returns device id, access token for account routes, refresh token, expires_at
  - access token here authorizes calls to the control plane, not to servers
- POST /v1/app/refresh
  - body: refresh token
  - rotates refresh token and returns a new access token
- POST /v1/app/logout
  - revokes a device/app session
- GET /v1/app/servers
  - authenticated by app access token
  - returns servers owned by the account with name, id, node_id, public_url, last_heartbeat_at, status
- POST /v1/app/token
  - body: server_id
  - returns a short-lived server-scoped token for that server
- GET /v1/app/skills
  - returns account skill metadata for the signed-in app
- GET /v1/app/skills/:id
  - returns one full account skill
- POST /v1/app/skills
  - creates an account skill from the app
- PUT /v1/app/skills/:id
  - updates an account skill with expected_version
- DELETE /v1/app/skills/:id
  - soft-deletes an account skill
- GET /v1/keys
  - public JWKS for server verification

## 8. Main flows

### 8.1 Signup and app sign-in

1. User enters email, password, and display name in the app or dashboard.
2. Worker validates and hashes the password.
3. App receives a refresh token plus a control-plane access token.
4. Refresh token is stored in OS secure storage: Keychain on desktop, Keystore on Android.
5. App fetches the server list from GET /v1/app/servers.

### 8.2 Add a new server with browser approval

1. On the new machine: console enroll.
2. CLI calls POST /v1/enroll/start with hostname and platform.
3. Worker creates an enrollment request and returns an approval URL.
4. CLI prints the URL and polls GET /v1/enroll/status every 2 seconds.
5. User opens the URL in a browser already signed into the dashboard.
   - If not signed in, login page first, then return to approval.
6. Approval page shows the requested name, platform, time, and approve/reject buttons.
7. On approve, Account DO creates a server record and returns server id and secret to the browser.
8. The browser shows the secret with a copy button and tells the user the CLI will finish automatically.
9. CLI poll sees approved, consumes the request, writes server id and secret to local storage, and prints success.

Important: the CLI does not need the user to copy the server secret from the browser. The secret is returned to the CLI by the poll response once. The browser copy path is only a fallback for UI debugging.

### 8.3 Add a headless server with a code

1. User opens the dashboard and clicks "Add server".
2. Dashboard calls POST /enroll-codes.
3. Worker creates a one-time code and shows it once.
4. User SSHes into the server and runs:
   console enroll --code K7MT-2Q9X-4BWN-R8QP
5. CLI calls POST /v1/enroll/code with the code, hostname, platform, and version.
6. Worker atomically consumes the code, creates the server record, and returns server id and secret.
7. CLI writes the credential locally and prints success.

Optional flag: --name overrides the auto-detected hostname.

### 8.4 App connects to a server

1. User selects a server from the app.
2. App calls POST /v1/app/token with server id.
3. Worker checks that the account owns the server and that the device session is active.
4. Worker returns a server-scoped token, lifetime 15 minutes.
5. App opens iroh/HTTPS connection directly to the server and sends the token as Authorization: Bearer.
6. Go server verifies token signature, audience server id, account id, device id, expiry, and key id.
7. Server maps account id + device id to a request-local identity. No local pairing table is needed for normal access.

### 8.5 Local fallback

Each enrolled server also creates a local access token on disk during console enroll or first server start.

Rules:

- Stored under ~/.console with 0600 permissions.
- Accepted only for requests where the peer is 127.0.0.1 or ::1.
- Never accepted over iroh, tunnel, LAN, or public URL.
- Used by desktop app when it is running on the same machine and the account service is unavailable.
- Can be shown/copied with console local-access.
- Can be rotated with console local-access --rotate.

This is not an account login. It is a machine-local admin escape path.

## 9. Sync: account skills

Skills are specialized instructions the model can load on demand. Today skills are discovered from user config directories and project directories on each server. That makes first setup simple, but daily use painful: the same skill has to be copied to every machine and kept there by hand.

This feature syncs the user-level `~/.agents/skills` skill library through the account. Project skills remain local to the repository. This is intended to work with skills installed by `npx skills` / skills.sh: once a skill lands under `~/.agents/skills`, the server detects it and syncs it automatically.

### 9.1 Source of truth

- The local source root is the user-level `~/.agents/skills` directory only.
- Do not sync project-local `.agents/skills`, `.console/skills`, or `.agent/skills` directories found inside repositories.
- The Account DO is the source of truth for account skills after upload.
- A skill record stores name, source path, frontmatter, full markdown body, checksum, version, and deleted_at.
- The full skill body is stored in the DO. Skills are prompt/workflow text, not secret storage.
- Every active skill has a unique normalized name. Project skills can still override an account skill with the same name when a run happens inside that project.
- Unknown frontmatter fields are preserved. A skill does not need to be "known" by Console in advance.
- Deletes are soft. When a local source skill disappears, the account skill is marked deleted with a 30-day undo window.

### 9.2 What gets synced

- Sync supported skill files under `~/.agents/skills`:
  - flat markdown files such as `~/.agents/skills/writing.md`
  - directory skills such as `~/.agents/skills/rust/SKILL.md`
  - markdown extensions `.md`, `.mdc`, and `.markdown`
- Ignore hidden files/directories, `.DS_Store`, `node_modules`, temporary files, and non-markdown files.
- Parse frontmatter for name/description/hide, but preserve the original raw body and unknown fields.
- If a file cannot be parsed, leave it local and log a warning; do not block syncing other skills.

### 9.3 Automatic upload and conflict rules

- The server watches or polls `~/.agents/skills` and uploads new or changed skills automatically.
- Local changes use last-write-wins for v1: a changed checksum uploads a new version and becomes the account copy.
- Skills can also be created, edited, and deleted from the hosted dashboard and from signed-in desktop/Android apps.
- When editing from dashboard/app, updates require expected_version. A stale version returns a conflict instead of silently overwriting another editor.
- Skill names are normalized for uniqueness but the original display name and source path are preserved.
- Local deletion soft-deletes the account skill with a 30-day undo window. This keeps accidental `npx` reinstall/removal operations recoverable.

### 9.4 Server mirror

- Each enrolled server includes synced_skills_revision in its heartbeat.
- When the control plane sees a newer skills_revision, heartbeat returns skills_changed.
- The server then calls GET /v1/sync/skills and atomically writes the bundle to a local mirror under `~/.console/sync/skills/<name>/SKILL.md`.
- Skill discovery adds `~/.console/sync` as a user-level skill root after the existing local user roots and before project roots.
- Precedence is: closest project skills win, then account-synced skills, then local user skills of the same name.
- Deleted account skills remove the mirrored file on the next sync.
- The server keeps the last successful mirror on disk. If the control plane is unavailable, existing skills continue to work.

### 9.5 CLI surface

- console skills status: show account connection, local source revision, mirror revision, and remote revision.
- console skills list: show account skill names, versions, and descriptions.
- console skills sync: run an immediate two-way sync check. Normal operation is automatic; this is only for debugging and explicit refresh.

## 10. Token design

Use compact custom JWT-like strings signed with ES256.

Claims:

    v: 1
    typ: app | server_access | local
    kid: signing key id
    iss: console-control-plane
    sub: account id
    device: device id
    server: server id
    token_version: account token version
    iat: unix seconds
    exp: unix seconds
    jti: random id

Lifetimes:

- account app access token: 15 minutes
- server-scoped token: 15 minutes
- refresh token: 30 days, rotated on use
- browser session cookie: 7 days
- enrollment request and code: 10 minutes
- local access token: no expiry, manually rotated

Server verification:

- Load public keys from /.well-known/console-keys.
- Cache keys on disk under ~/.console/control-plane-keys.json.
- Refresh in the background; do not block requests on Cloudflare.
- Reject unknown kid, bad signature, wrong typ, wrong server, expiry, and token_version mismatch.
- Heartbeat returns the current expected token_version. If a token has a stale version, the server rejects it.

Revocation behavior:

- Revoke one device: remove app session and bump token_version.
- Revoke one server: mark server revoked and bump token_version.
- Revoke all devices: bump token_version once.
- Because tokens are verified offline, revocation takes effect as soon as the server sees the new heartbeat response, or when the 15-minute token expires. This trade is intentional.

## 11. Go server changes

New package: internal/cloudauth

Responsibilities:

- Store control-plane config, server id, server secret, and cached public keys.
- Run enrollment commands.
- Heartbeat to the control plane.
- Verify server-scoped access tokens.
- Produce and verify local access tokens.

Files:

    internal/cloudauth/config.go
    internal/cloudauth/enroll.go
    internal/cloudauth/heartbeat.go
    internal/cloudauth/token.go
    internal/cloudauth/local.go
    internal/cloudauth/keys.go
    internal/cloudauth/skills.go

CLI commands:

- console enroll [--name NAME] [--code CODE]
- console local-access [--rotate]
- console account-status
- console skills status|list|sync
- console enroll --reset maybe later, only if needed

Changes in existing files:

- internal/cli/root.go: add commands.
- internal/cli/daemon/config.go: add control_plane_url, server_id, and enrollment state fields.
- internal/routes/routes.go: replace RequireDeviceToken with a combined middleware:
  - local token accepted only on loopback
  - Cloudflare-signed server token accepted over any connection
- internal/routes/device_auth.go: remove or retire old pairing routes as primary API. Keep only if needed for local debug during transition.
- internal/serve/serve.go: start heartbeat goroutine after successful enrollment; do not block boot if unenrolled.
- internal/db/schema.go: remove or ignore old auth tables for primary auth. Do not migrate old pairings.
- internal/services/skills_service.go and internal/agent/systemprompt/skills.go: add ~/.console/sync as a user-level skill root after existing local user roots and before project roots, so account-synced skills override local user skills but project skills still win.
- internal/cloudauth/skills.go: watch or poll `~/.agents/skills`, parse supported markdown skill files, upload changed checksums automatically, and mirror account bundles into `~/.console/sync/skills`.

Server startup behavior:

- If not enrolled: server still starts, but all API requests except local loopback and enrollment status return a clear "server is not enrolled" error.
- If enrolled: server starts normal API, loads keys from disk, starts heartbeat.
- If Cloudflare is unreachable: server uses cached keys and continues serving existing tokens.

## 12. Desktop and Android changes

Desktop:

- Add "Sign in to Console" to settings/onboarding.
- Securely store refresh token.
- Replace manual host/pairing-code server add flow with account server list.
- Keep manual host entry only for advanced/local development.
- Fetch server list from control plane on sign-in and refresh.
- For each selected server, obtain a server-scoped token and set it on HttpTransport.
- Add local fallback detection: if a server is localhost and Cloudflare is unreachable, use local access token from disk or prompt for it.
- Add account skills management: list, create, edit, delete, show version conflicts, and display local source/mirror status.

Android:

- Same model with email/password sign-in.
- Store refresh token in Android Keystore-backed storage.
- Server list comes from control plane.
- Obtain server-scoped tokens before connecting.
- No manual pairing code in the main path.
- Add account skills management with the same version-conflict rules as desktop.

## 13. Proto and client API changes

Add proto/console/v1/account.proto for shared client types:

- AccountProfile
- ConsoleServer
- ConsoleDevice
- AccountSettings
- AccountSkill
- SkillBundle
- SignInRequest/Response
- ServerTokenRequest/Response
- EnrollmentStatus maybe only CLI uses JSON

Do not reuse provider AuthStatus types. Provider accounts remain separate and unchanged.

## 14. Web dashboard

Need at least a minimal hosted web UI for:

- signup with required display name, login, logout
- account profile
- list servers/devices
- create headless enrollment code
- approve/reject enrollment request
- rename/revoke server/device
- account settings editor
- account skills editor with create, edit, delete, and conflict messages

This can live inside apps/control-plane/web initially. It does not need to be a large SPA. Keep it server-rendered or a tiny client bundle.

The browser approval page must show:

- requester name/platform
- requested server name
- request age and expiry
- approve/reject buttons
- signed-in account email

## 15. Security requirements

- Password hashing with PBKDF2-SHA256, per-user salt, and high iteration count.
- Rate limit login, signup, enrollment start, and enrollment code redemption.
- Secure cookies: httpOnly, secure, sameSite=Lax, short-lived; CSRF token for browser POST routes.
- App refresh tokens rotate on use and are hashed at rest.
- Server secrets are random 256-bit values, hashed at rest, shown only once.
- Enrollment codes use a human-safe alphabet with at least 80 bits of entropy.
- All secret-bearing responses set no-store headers.
- Do not log request bodies for auth/enrollment routes.
- Audit log entries for signup, login, enrollment approve, code creation, server revoke, device revoke.
- Skill bodies are account data, not secret storage. Audit logs record create/update/delete events but never skill bodies.
- Control plane stores no chat/file/provider secret data.

## 16. Verification

Control plane:

    cd apps/control-plane && npm install
    cd apps/control-plane && npm test
    cd apps/control-plane && npx wrangler dev

Manual local checks:

- signup with display name, login
- create code, redeem code with a fake CLI call
- start browser enrollment, approve, poll status
- sign app in, list servers, request server token
- verify token shape and signature with a local server
- create a skill, pull it on a server, run an agent, and confirm the skill is available
- edit the same skill from a second device with a stale version and confirm the conflict is reported

Go:

    cd apps/server-go && go test ./tests/api/ -run Enrollment -v
    cd apps/server-go && go test ./tests/api/ -run CloudAuth -v
    cd apps/server-go && go test ./tests/api/ -run SkillsSync -v
    cd apps/server-go && go vet ./...

Desktop:

    cd apps/desktop && cargo test -p console-core account
    cd apps/desktop && cargo test -p console-core server_auth
    cd apps/desktop && cargo check --locked --manifest-path Cargo.toml

Android:

    cd apps/android && ./gradlew :app:assembleDebug
    cd apps/android && ./gradlew :app:testDebugUnitTest --tests "<relevant fixture test>"

## 17. Implementation phases

### Phase 1: Control plane only

Deliverable: Worker plus DOs plus minimal web pages. Can sign up, log in, create codes, approve requests, list servers/devices, issue tokens, serve keys, and create/edit/delete account skills. No Go/client integration yet.

### Phase 2: Go server enrollment and verification

Deliverable: console enroll works with both modes. Go server stores config, heartbeats, verifies Cloudflare-signed tokens, syncs account skills into the local mirror, and has a local fallback token.

### Phase 3: Desktop account sign-in

Deliverable: desktop can sign in with email/password, list servers, connect using server-scoped tokens, and manage account skills.

### Phase 4: Android account sign-in

Deliverable: Android parity for sign-in, server list, server-scoped tokens, and account skills management.

### Phase 5: Cleanup

Remove old pairing as the default path. Update roadmap, README, install docs, and onboarding copy. Keep only the local fallback path.

## 18. Open decisions to settle during implementation

- Public hostnames: control-plane API domain and dashboard domain.
- Password hashing uses PBKDF2-SHA256 via WebCrypto.
- Exact enrollment code length and alphabet.
- Whether server token lifetime stays at 15 minutes.
- Whether local fallback can read a token from disk automatically or must be pasted into the app once.

## 19. Definition of done

A user can sign up with email/password on a fresh machine, enroll a server with either browser approval or a one-time code, sign into a second device with the same account, see the server automatically, connect to it without pairing, install or edit skills under `~/.agents/skills` that sync automatically to every enrolled server, revoke a device or server from the dashboard, and still access a local server from localhost if the control plane is unavailable.