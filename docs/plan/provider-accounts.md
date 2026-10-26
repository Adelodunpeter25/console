# Provider accounts — multiple accounts per provider with an `active` flag

Adds **explicit multi-account support**: log into one or more accounts per
provider, see exactly which one is in use (`active: true`), switch between
them, and remove the ones that are no longer wanted. Inline from the roadmap:
"Multi-account support — log into multiple accounts per provider and switch
between them; `?active` marks the account in use" (`roadmap.md:19`).

This is **explicit switching only**. There is no rotation, no load balancing,
no cooldown, no failover. CLIProxyAPI was reviewed at length and rejected as a
dependency — it is a pooling gateway, and the pooling machinery is exactly what
is *not* wanted here. What is worth keeping from it is the shape of its
credential management: one credential per file, an id/provider/label record
with derived status, and list/toggle endpoints. That is the reference. The code
is not copied; none of its packages are imported.

Target: `apps/server-go/internal/accounts/` (new store), `internal/db/schema.go`
(table), `internal/providers/{claude,codex,antigravity}/oauth.go`
(account-aware credentials), `internal/auth/auth.go` + `internal/routes/auth.go`
(API), `internal/agent/loop/loop.go` + `internal/run/` (pin the account per
run), `proto/console/v1/auth.proto`, `apps/desktop/crates/console-ui/src/settings/providers_page.rs`,
`apps/android/.../feature/settings/ProvidersSettings.kt`, and
`apps/android/rust-wt`-style fixture tests under `proto/testdata/auth/`.

Legend: ✅ done and verified · ◐ done, not verified · ⬜ not done.

| Phase | State | Notes |
| :--- | :--- | :--- |
| 0 — decisions | ✅ | §0 |
| 1 — account store | ⬜ | schema, migrate, tests |
| 2 — provider wiring | ⬜ | account-aware load/save/refresh |
| 3 — API + proto | ⬜ | status.accounts, activate, delete |
| 4 — run pinning | ⬜ | resolve once per run |
| 5 — desktop UI | ⬜ | account list per provider |
| 6 — Android UI | ⬜ | account list per provider |
| 7 — deferred | — | per-account usage, OpenCode keys, Devin |

---

## 0. Decisions (locked)

These settle the product questions before any code is written. They are
decisions first and rationale second; a phase that disagrees with a decision
here is wrong.

1. **Explicit switching, never rotation.** The user picks which account runs
   use by setting `active`. No automatic selection exists in v1. This is the
   roadmap's wording ("switch between them"), and it is the reason a
   CLIProxyAPI-style gateway is out of scope: the gateway *hides* which account
   served a request, which is the opposite of what is needed.
2. **At most one active account per provider.** `active: true` on one clears
   the others in a single transaction. Zero active is a legal state that means
   "credentials exist but nothing is in use" — runs fail with a clear error
   naming the provider, and the UI shows the row as needing a selection. This
   keeps `active` honest as a boolean parameter rather than as sugar for
   "first enabled".
3. **Adding an account does not steal `active`.** A newly added account becomes
   active only when the provider had no active account (or none at all).
   Without this rule, adding a second account mid-chat would silently switch
   the provider under existing runs. The UI offers an explicit Use affordance.
4. **Deleting the active account promotes the next one.** Deterministic order:
   by `created_at`, oldest first. If no accounts remain, the provider returns
   to "logged out". Deactivating the *only* active account via `active:false`
   is allowed (decision 2's zero-active state) — the server warns in the
   response rather than refusing, since removing credentials is the other way
   to reach the same state.
5. **Dedupe by provider identity, not by file.** Re-login with the same account
   updates tokens in place. Identity per provider: Claude — email (JWT bootstrap);
   Codex — `chatgpt_account_id` falling back to email; Antigravity — email, with
   the project id already carried inside the credential. If a provider cannot
   determine an identity, a fresh account is created (never merged).
6. **Secrets stay in files; metadata goes to SQLite.** Same convention as
   today: credential payloads are 0600 files under
   `<storage>/accounts/<provider>/<id>.json`; the account list and the active
   pointer live in the global DB (`utils/paths.go:87`) for atomic activation.
   Copying tokens into SQLite would also put secrets in a file that backups,
   worktrees and tooling treat differently from `.console` configs.
7. **The account is pinned per run.** `active` is resolved once when a run
   starts and used for that run's turns, including subagents. Changing
   `active` affects new runs only. Mid-run reads of a mutable pointer would
   switch providers between turns, invalidating prompt caching and failing
   replayed reasoning content (Claude thinking blocks replay unsigned per
   provider; see `providers/claude/convert.go:127`).
8. **Refresh is single-flight per account.** Two concurrent refreshes on one
   credential can both write; with one account per file this is already a
   latent race and multi-account widens it. Keyed mutex on
   `(provider, accountID)`.
9. **Environment tokens keep precedence and are surfaced as an account.**
   `CLAUDE_OAUTH_TOKEN`/`ANTHROPIC_OAUTH_TOKEN` already win over files
   (`providers/claude/oauth.go:204`). The API reports a synthetic account with
   id `env`, `active: true`, `env: true` and no `Delete` — deactivating it is
   meaningless while the env var is set, so the response says so. No behavior
   change, no mystery.
10. **New accounts use `ConsoleStorageDir()`.** The existing provider helpers
    pin `~/.console` regardless of dev mode (`providers/shared/oauth.go:94`,
    and the comment on `utils/paths.go:91` calls this out as deliberate for
    GitHub). Accounts go under the storage dir, so dev/prod and
    `CONSOLE_STORAGE_DIR` stay isolated. Migration reads the legacy pinned
    paths so production users carry over.

---

## 1. Non-Goals

- **No CLIProxyAPI code, module, or process.** Not a dependency, not a sidecar,
  not vendored. Its `auth` package is also not extractable for this use:
  `sdk/cliproxy/auth/types.go:20` imports Gin, and the selectors import
  `internal/thinking`, `internal/credentialweight` and `sdk/cliproxy/executor`.
  The reference is its *shape* (§3), reimplemented.
- **No rotation, cooldown, weighting, or failover.** One `active` pointer is
  the entire selection algorithm.
- **No per-session account choice.** Sessions use the run-pinned account from
  the provider's active-at-run-start value. A session-level picker is a later
  add-on on top of the same store, not part of v1.
- **No OpenCode multi-key in v1.** Its credential is a pasted API key in
  `provider-keys.json` (`providers/apikeys/apikeys.go:53`), not an OAuth
  account. Multi-key support would ride the same store but is deferred to keep
  v1 OAuth-shaped.
- **No per-account usage in v1.** Scope/types already carry `AccountID`
  (`providers/usage/types.go:21`) but the wire does not yet
  (`proto/console/v1/usage.proto:41` has no `account_id`). Looping accounts
  through `usage/service.go:124` is a later phase.
- **No Devin.** Still reserved and answering 501 (`routes/auth.go:52`); the
  store is provider-agnostic so this is free to add later.
- **No credential contents over the wire.** Same rule as today
  (`proto/console/v1/auth.proto:23`): tokens never cross the API boundary.

---

## 2. What Already Exists (verified)

- **One credential file per provider, written 0600 in a 0700 dir.**
  `providers/shared/oauth.go:94` (`CredentialPath` → `~/.console/<name>`,
  env override), `:104` (`SaveCredentialFile`), `:116`
  (`CredentialFileExists`).
- **Status/login endpoints already exist and stay.** `POST /api/auth/login/url`
  and `/login/callback` (`routes/auth.go:31`, `:59`) drive PKCE logins for
  codex/claude/antigravity via `auth.AuthService` (`internal/auth/auth.go:113`
  `GetStatus`, `:182` `GetLoginURLFor`, `:244` `HandleCallbackFor`, which saves
  the exchanged credential straight to the single path today).
- **The proto already models per-provider auth and tolerates additions.**
  `proto/console/v1/auth.proto:28` `ProviderAuthStatus`,
  `:50` `AuthStatusResponse`, with a documented convention that clients treat
  unknown provider ids as "not reported" (`console-core/types/auth.rs:37`)
  and protojson *drops* implicit-presence fields — so new fields must be
  explicit-presence or repeated.
- **Providers are stateless and credential-driven.** `providers.Lookup`
  (`providers/registry.go:16`) returns a fresh provider per request; the
  provider loads its own credential inside `RunTurn`
  (`claude/stream.go:46` `loadCredential` → `LoadCredential` →
  `RefreshIfNeeded`, `claude/oauth.go:204`/`:245`). Adding an account id to the
  load path touches only the load function.
- **Per-account identity already exists inside credentials.** Codex carries
  `AccountID = chatgpt_account_id` (`providers/codex/oauth.go:25`) and requires
  it (`:200`); Antigravity carries its own `projectId`
  (`providers/antigravity/oauth.go:24`); Claude carries `Email` fetched at
  exchange time (`claude/oauth.go:99`). Multi-account is mostly *referring to*
  these identities, not inventing new ones.
- **Run options are already forwarded per turn.** The loop's per-run knobs
  (`Model`, `ConversationID`, `ThinkingLevel`, `CacheRetention`) live on
  `Agent` (`agent/loop/loop.go:191-194`) and are copied into `TurnRequest`
  built at `:380`. A pinned account id rides the same rails.
- **Model caches invalidate on logout already.**
  `claude/models.go:36` and `antigravity/models.go:40` document
  `InvalidateModelCache` being called on logout; the same hooks fire on
  activate/add/delete.
- **DB DDL is idempotent with additive migrations**
  (`internal/db/schema.go:1`), so a new table plus an index is business as
  usual. The global DB is the right home (devices/pairing live there too).
- **GitHub is the template for a non-OAuth credential flow.** Validate with a
  live probe before write, `POST /api/auth/github/logout` idempotent
  (`routes/auth.go:188`, `tests/api/github_auth_test.go:189`). Account delete
  follows the same idempotent rule.
- **Android and desktop render from the provider catalog / auth status, not
  from a hardcoded provider list.** `ProvidersSettings.kt:83` filters by
  `auth_method` and maps each provider to a status; desktop
  `providers_page.rs:60` renders rows from the catalog. The UI work is
  additive per provider, not a redesign.

---

## 3. Reference — what to borrow from CLIProxyAPI

**Borrow (shapes and rules, not code):**

| Pattern | Where it lives there | Console equivalent |
| :--- | :--- | :--- |
| One credential per file | `auths/*.json`, dir per auth-dir config (`config.example.yaml:924`) | `accounts/<provider>/<id>.json` |
| Record: id, provider, label, email, status, timestamps | `sdk/cliproxy/auth/types.go:52` (only those fields) | `accounts.Account` (§4) |
| List endpoint with per-entry status | `management/auth_files.go:106` `ListAuthFiles` | `accounts` array on status |
| Toggle endpoint returning the new state | `management/auth_files_fields.go:29` `PatchAuthFileStatus` | `PATCH /api/auth/accounts/:provider/:id` |
| Status is *derived* from credential health, never stored as truth | `auth_files.go:493` `buildAuthFileEntry` reconciliation | `loggedIn`/`email` derived from active credential file each status read |
| Stable file-name identity used for lookup and mutations | `authFileListName`/`lookupAuthFile` | account id + provider path |

**Explicitly do not borrow:** `Disabled` plus the selector scheduler,
`Quota`/`ModelStates`, `Priority`/`Weight`, session affinity, the conductor,
per-auth transports, the file watcher, and the plugin/executor registration.
All of that exists to pool credentials, which has been ruled out (decision 1).

---

## 4. Data model

```go
// internal/accounts/account.go
type Account struct {
    ID        string    // "acc_" + 16 hex chars; stable, stored, never derived
    Provider  string    // "claude" | "codex" | "antigravity" (open set on the wire)
    Email     string    // display identity; may be empty (§4.1)
    Label     string    // optional operator label, else server derives from Email
    Active    bool      // derived from the provider's active pointer on read
    CreatedAt int64     // unix ms
    UpdatedAt int64     // unix ms
}
```

### 4.1 Identity (dedupe key)

| Provider | Identity key | Fallback |
| :--- | :--- | :--- |
| Claude | email (JWT bootstrap) | — (no email → new account) |
| Codex | `chatgpt_account_id` | email |
| Antigravity | email | — (project id is per-account but not a stable cross-restart key) |

Dedupe happens at login-callback time: a match updates that credential file in
place (`CredentialVersion` bump semantics are unnecessary — the file *is* the
credential), a non-match creates a new account.

### 4.2 Store

SQLite (`internal/accounts/store.go`), via the existing global DB:

```sql
CREATE TABLE IF NOT EXISTS provider_accounts (
  id         TEXT PRIMARY KEY,
  provider   TEXT NOT NULL,
  email      TEXT NOT NULL DEFAULT '',
  label      TEXT NOT NULL DEFAULT '',
  active     INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_provider_accounts_provider
  ON provider_accounts(provider, created_at);
-- The invariant is enforced by the DB, not by hope.
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_accounts_one_active
  ON provider_accounts(provider) WHERE active = 1;
```

Activate/deselect are single transactions:

```sql
UPDATE provider_accounts SET active = 0, updated_at = ? WHERE provider = ? AND active = 1;
UPDATE provider_accounts SET active = 1, updated_at = ? WHERE id = ? AND provider = ?;
```

Two statements, one transaction, partial unique index making double-active
impossible. Delete-active promotion (decision 4) is the same transaction shape
plus an insert-if-nothing-was-activated step.

### 4.3 Credential files

```
<ConsoleStorageDir()>/accounts/claude/acc_01.json      # same OAuthCredential JSON as today
<ConsoleStorageDir()>/accounts/codex/acc_02.json
<ConsoleStorageDir()>/accounts/antigravity/acc_03.json
```

Path helper `accounts.CredentialPath(provider, id)`. Write via the existing
`shared.SaveCredentialFile` (mkdir 0700, file 0600, atomic rename to be added
as part of phase 2 since refresh now writes to a *directory* of accounts).

### 4.4 The synthetic env account

When `CLAUDE_OAUTH_TOKEN`/`ANTHROPIC_OAUTH_TOKEN` (and equivalent per provider)
is set, status reports exactly one account: `{id: "env", env: true, active: true}`.
It cannot be removed or deselected while the env var is set; the delete/patch
responses say so in their error. File accounts stay in the list, inactive,
harmless — precedence is env-first as today (`claude/oauth.go:204`).

---

## 5. Wire (proto) + API

### 5.1 Proto additions (additive, explicit presence)

```proto
// One stored login. Secrets never cross the wire (auth.proto
// comment block). `active` is always sent (explicit presence) so a reader can
// distinguish "this account is in use" from "field absent".
message ProviderAccount {
  string id = 1;
  optional string email = 2;
  optional string label = 3;
  bool active = 4;
  int64 created_at = 5;
  bool env = 6;             // synthetic env-token account (§4.4)
}

message ProviderAuthStatus {
  bool logged_in = 1;
  optional string email = 2;
  optional string project_id = 3;            // active credential's project (antigravity)
  optional string configured_project_id = 4; // settings-level override
  repeated ProviderAccount accounts = 5;     // NEW: every stored account
}

message AccountMutationResponse {
  string provider = 1;
  ProviderAuthStatus status = 2;  // the provider's post-mutation state
}
```

`AuthStatusResponse` and the login messages stay as they are, except
`OAuthCallbackResponse` gains `optional string account_id` and
`optional bool account_active` so a successful add tells the client what
happened without a second round trip.

### 5.2 Status semantics

`loggedIn` / `email` continue to describe the **active** account, exactly as
they describe the sole account today. Old clients (and old testdata) keep
working unchanged: they read `logged_in`/`email` and ignore `accounts`.

### 5.3 Routes

| Method | Path | Body / params | Notes |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/auth/status` | — | now includes `accounts` per provider |
| `POST` | `/api/auth/login/url` | `{provider}` | unchanged |
| `POST` | `POST /api/auth/login/callback` | `{provider, code, state}` | unchanged shape; now creates/updates an account (§5.4) |
| `PATCH` | `/api/auth/accounts/:provider/:id` | `{active?: bool, label?: string}` | activate or deselect; per decision 2/4 |
| `DELETE` | `/api/auth/accounts/:provider/:id` | — | idempotent; promotes next active if needed |

Server-side rules encoded in the handlers, echoing the decisions:

- Unknown provider id → 400 (same wording/style as the existing switch in
  `routes/auth.go:42`).
- Unknown account id → 404.
- `active:true` on an already-active account → 200, no-op (idempotent like the
  GitHub logout route).
- `active:false` on a non-active account → 200, no-op.
- Deleting the `env` account → 409 with the env-precedence explanation.
- Every mutation invalidates that provider's model/usage caches (§6.3) and
  returns `AccountMutationResponse`, so the client updates from one response.

### 5.4 Login flow (unchanged verbs, new destination)

Today `HandleCallbackFor` saves to the single path. It becomes:

1. Exchange code → credential (existing provider code, untouched).
2. Compute identity (§4.1).
3. Store: match existing account → overwrite its credential file; else create
   row + write file.
4. Activate the new account only if the store has `COUNT(active)=0` for this
   provider (decision 3).
5. Return `OAuthCallbackResponse` with `account_id` and `account_active`.

The pending-state map (`auth.go:29`) is unchanged; a second concurrent login
for the same provider simply produces two accounts (dedupe decides), not a
lost credential.

---

## 6. Server wiring

### 6.1 The `internal/accounts` package

Package-level, like `providers/apikeys` (no DI churn in constructors):

```go
// internal/accounts
func Configure(db *db.DB) error                 // DDL is in db/schema.go; store opens the table
func List(provider string) ([]Account, error)
func All() (map[string][]Account, error)
func Enabled(provider string) (Account, bool, error)   // active account, if any
func EnabledID(provider string) (string, error)        // "" when none
func CredentialPath(provider, id string) string        // <storage>/accounts/<provider>/<id>.json
func Activate(provider, id string) error
func Select(provider, id string, active bool) error
func Delete(provider, id string) error                 // idempotent, promotes next (decision 4)
func Register(provider string, identity, email, label string) (Account, bool, error)
// identity helpers shared by providers:
func IdentityFor(provider string, cred any) string     // §4.1 per-provider key
```

`routes.go` calls `accounts.Configure(cfg.DB)` once at boot next to the other
service constructions (`routes.go:71-137`). Tests call `Configure` with a temp
DB.

### 6.2 Account-aware credentials (per provider)

Each OAuth provider gains an explicit form; the legacy function stays:

```go
// providers/claude/oauth.go
func LoadCredentialFor(accountID string) (ParsedCredential, error) // empty id = active account (or legacy file)
func SaveCredentialFor(accountID string, cred OAuthCredential) error
```

- `LoadCredentialFor("")` = `accounts.EnabledID("claude")`; empty result falls
  back to the legacy `claude-creds.json` path. This is what keeps phase 1–4
  independent: until run pinning lands, every provider behaves exactly as
  today, transparently served from the migrated account file.
- Env token wins first, as today (`claude/oauth.go:204`), before any file read.
- `RefreshIfNeeded` writes back via `SaveCredentialFor(accountID)`.
- `internal/providers/shared` gains a keyed single-flight:
  `shared.RefreshSingleFlight(provider, scope string)` wrapping refresh+save
  (decision 8). `scope` is the account id (or `"legacy"` / `"env"`).
- Codex `RefreshIfNeeded` currently writes back directly
  (`providers/codex/oauth.go` path handling); codex and antigravity adopt the
  same `For` functions so all three shapes match.

### 6.3 Cache invalidation on mutations

`Activate`/`Select`/`Delete`/+login succeed → invalidate model discovery and
the usage cache for that provider (the `InvalidateModelCache` hooks noted in
`claude/models.go:36` and `antigravity/models.go:40`, plus
`usage.Service.Invalidate(provider)`). Model availability can differ by account
plan; the picker should refresh rather than serve a stale union. In v1 the
catalog is the union across accounts; filtering per account is deliberately
deferred (§11) and run-time resolution surfaces "model not available" as an
upstream error the client already renders.

---

## 7. Run wiring — pin the account (decision 7)

- `TurnRequest` (`agent/loop/loop.go:29`) gains `AccountID string` alongside
  `BaseURL`.
- `Agent` (`:170`) gains `AccountID string` as another forwarded run option;
  the internal `TurnRequest` build at `:380` copies it.
- `run` resolves once per run: in `Service.execute`
  (`run/turns.go:44`) and for every provider construction
  (`run/service.go:105` resolves `providers.Lookup(providerID)`), the run
  store reads `accounts.EnabledID(providerID)` **once**, stores it on the
  activeRun, and passes it to the loop and to the nested surfaces it
  constructs (subagent path `NewSubagentTool`/`SubagentContext`,
  `run/turns.go:378`; compaction and titles read the same field).
- Failure cases, spelled out:
  - No active account and no legacy credential → the provider's existing
    "Please login again" class of error, surfaced as today's turn failure.
  - Active account deleted mid-run (another tab/client) → the next turn fails
    with `account <id> was removed; select another account for provider <provider>`.
    **No silent fallback to another account.** A mid-run switch invalidates
    prompt cache and can break replayed thinking; failing loud is the
    documented behavior (fallthrough of decision 7).
  - `enabled` account's refresh fails hard → existing unauthorized-refresh
    error path; the row shows the failure in the UI through status.

---

## 8. Migration

`accounts.Migrate()` runs from `Configure`, guarded and idempotent:

1. For each of `claude`, `codex`, `antigravity`: if the provider has **zero**
   rows and its legacy file exists
   (`~/.console/claude-creds.json` etc., the env-overridden path from
   `shared.CredentialPath`), create one account `id = "acc_legacy_" + provider`,
   `active = 1`, and **copy** the file to
   `accounts/<provider>/<id>.json`.
2. The legacy file is left in place for one release (rollback safety). The
   legacy `LoadCredential` fallback in §6.2 disappears in the same release that
   deletes the file — both documented in the phase notes, neither silent.
3. Migration is also what repairs env-token users transparently: they get an
   `env` account row locked to `active` until the env var is removed.

`Migrate` is covered by the phase-1 tests with both present and absent legacy
files, dev-dir vs storage-dir, and a pre-existing active row (must not add a
second active: the partial unique index failing loudly is the test).

---

## 9. Clients

### 9.1 Desktop (`apps/desktop/crates/console-ui`)

- `console-core` regenerates against the new proto; `OAuthProviderId` and
  `auth_status_for` (`console-core/types/auth.rs:44`) stay — they read the same
  fields as today.
- `settings/providers_page.rs:` each OAuth provider row gains an expandable
  account list: email/label, an Active badge, **Use** (PATCH) and **Remove**
  (DELETE with confirm). "+ Add account" triggers the existing login flow; on
  callback success it refreshes status from the response's `account_id` and
  highlights the new row when it is *not* active (decision 3's visible-use
  affordance).
- Zero-active state renders the row as "No account selected — choose one" while
  keeping the credential count visible.
- No rename UI in v1 (label is derived from email); the API supports labeling
  for later.

### 9.2 Android (`apps/android`)

- `AuthRepository.kt:188` maps the status proto; add `ProviderAccount` decoding
  (`accounts` field) into `AuthState`.
- `ProvidersSettings.kt` renders the account list under each provider row with
  the same Use/Remove/Add affordances; `ApiKeyProviderRow.kt` is untouched.
- Fixture tests: extend `proto/testdata/auth/status.json` with accounts **and
  add a second legacy fixture without the field** to pin that old payloads
  still decode (field is repeated, so this is guaranteed by protojson, but the
  fixture makes it a regression test, not an assumption).
- Per `AGENTS.md`: no test functions in `app/src/main`; fixtures live in
  `apps/android/tests/*FixtureTest.kt` decoding `proto/testdata`; Android tests
  are not CI-enforced.

---

## 10. Phases and verification

Each phase is independently shippable. Commands follow `AGENTS.md`: one test
file or one crate at a time; never the full suite.

### Phase 1 — account store (⬜)

- `internal/db/schema.go`: table + partial unique index (§4.2).
- `internal/accounts/`: `Account`, `Configure`/`Migrate`, `List`/`All`,
  `EnabledID`, `Activate`/`Select`/`Delete`, `Register`, path helpers (§4/§8).
- `routes.go`: single `accounts.Configure(cfg.DB)` call.
- Tests: `tests/providers/accounts_test.go` — CRUD, unique-active invariant
  (DB rejects double-active), deactivate-to-zero, delete-active promotion,
  migration with/without legacy files, dev vs storage dir `CredentialPath`.
  Run: `cd apps/server-go && go test ./tests/providers/ -run TestAccounts -v`.

### Phase 2 — account-aware providers (⬜)

- `providers/shared`: `SaveCredentialFile` atomic rename; refresh
  single-flight.
- claude/codex/antigravity: `LoadCredentialFor`/`SaveCredentialFor`, env-first
  precedence, identity exposure for §4.1, legacy path fallback.
- Tests: per-provider load/save/refresh roundtrips on two accounts +
  single-flight race test. Run per provider, e.g.
  `cd apps/server-go && go test ./tests/providers/ -run TestClaudeAccounts -v`.

### Phase 3 — API + proto (⬜)

- `proto/console/v1/auth.proto`: §5.1 additions; `make proto`; regen `internal/gen`.
- `internal/auth`: `GetStatus` reads `accounts.All()`; callback uses
  `Register` + activate-if-none (§5.4).
- `internal/routes/auth.go`: `PATCH`/`DELETE /api/auth/accounts/:provider/:id`,
  `AccountMutationResponse` mapping, error codes (§5.3).
- Tests: `tests/api/auth_accounts_test.go` — status includes accounts legacy
  (no-accounts fixture decodes), add via callback, activate → deactivate →
  delete, dedupe on same-identity re-login, delete env → 409, delete-active
  promotion reflected in the returned status.
  Run: `cd apps/server-go && go test ./tests/api/ -run TestAuthAccounts -v`.
- Fixtures: `proto/testdata/auth/status.json` (with accounts) + a new
  `status_no_accounts.json`; Android `AuthAccountsFixtureTest.kt`;
  run `cd apps/android && ./gradlew :app:testDebugUnitTest --tests "AuthAccountsFixtureTest"`.

### Phase 4 — run pinning (⬜)

- `TurnRequest.AccountID`, `Agent.AccountID`, copy in the internal build
  (`loop.go:380`); `run` resolves `EnabledID` once per run and threads it into
  the loop and `SubagentContext` (`run/turns.go:378`).
- Tests: `tests/agent/run_test.go` extension — pinned account used across
  turns and subagents; active-account switch mid-run does **not** change the
  run; deleted account fails the next turn with the §7 message.
  Run: `cd apps/server-go && go test ./tests/agent/ -run TestRunAccountPin -v`.

### Phase 5 — desktop UI (⬜)

- `console-core` regen; `providers_page.rs` account list + Use/Remove/Add; bind
  to `AccountMutationResponse` updates.
- Verify: `cd apps/desktop && cargo check -p console-core -p console-ui`
  then the manual flow on `make dev-server` + client (docs carry no UI tests;
  add `tests/api` coverage already exists server-side).

### Phase 6 — Android UI (⬜)

- `AuthRepository`/`AuthState` account mapping; `ProvidersSettings.kt` rows.
- Verify: `cd apps/android && ./gradlew :app:assembleDebug` then
  `./gradlew :app:testDebugUnitTest --tests "*FixtureTest"` (only the touched
  fixture tests).

---

## 11. Deferred (documented, not built)

1. **Per-account usage.** Loop accounts in `usage/service.go:124`, plus
  `optional string account_id` on `UsageScope` (`usage.proto:41`) and UI
  labels. The Go types already carry `AccountID`, so the server half is
  plumbing; the proto and both clients' usage screens are the work.
2. **OpenCode multi-key.** Migrate `provider-keys.json` into the store as
  accounts with label; `ApiKeyProviderRow` gains a list. Same routes, no OAuth
  deltas.
3. **Label editing.** API supports `label`; UI edit ships when a real device
  workflow needs it.
4. **Session-level account pick.** Over rides run pinning; needs run-service
  plumbing like `thinking_level` has (`types/session.go`).
5. **Background token refresher.** v1 refreshes on use
  (`RefreshIfNeeded`, 5-minute skew). A boot-time sweep that reports refresh
  failures in status is a nice-to-have; single-flight makes it safe to add.
6. **Devin.** When it lands, it registers against the same store; the routes'
  provider switch (`routes/auth.go:42`) is the only place to extend.
