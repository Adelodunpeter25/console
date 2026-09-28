# MCP Client Support — Implementation Plan

Adds first-class **MCP client** capability to the agent: the harness connects out to
user-configured MCP servers (remote HTTP, or local stdio processes) and exposes their
tools to the model alongside the existing native tools in
`apps/server-go/internal/agent/tools`. This is a client integration only — console
does not ship an MCP server.

**Primary target server**: [Atlassian's official remote MCP server](https://github.com/atlassian/atlassian-mcp-server)
(`https://mcp.atlassian.com/v2/mcp`, Jira/Confluence/Bitbucket/etc.). It's remote
HTTP, authenticates via **OAuth 2.1** (or API token as a headless fallback), and
already does its own server-side lazy tool discovery (exposes a small "primary" set,
the rest discoverable on demand — same idea Task 4.4 `lazy_tools` already covers on
our side). That shapes the priority order below: remote HTTP + OAuth ships before
stdio, because that's what the flagship use case actually needs.

Target layout: `apps/server-go/internal/services/mcp/` (as built)

## Implementation status

Phases 1–5 are implemented (built on the official `modelcontextprotocol/go-sdk`); Phase 6
is partly done (HTTP API only, no desktop UI yet).

- `config.go` / `credentials.go`: `~/.console/mcp-servers.json` and `mcp-credentials.json` (0600).
- `client.go`: streamable HTTP and stdio transports, tool list/call, static Authorization header.
- `oauth.go` / `browser.go`: SDK `AuthorizationCodeHandler` (discovery, dynamic registration, PKCE),
  loopback callback listener, browser opener, token persistence + silent refresh.
- `manager.go`: lazy per-server connection, status (`needs_auth` exposes `authUrl`), reconnect on demand.
- `adapter.go`: `mcp__<server>__<tool>` tools, tier resolution (override, readOnly hint, name prefix,
  default write) and the `loadTools` tool (`group: "mcp:<serverId>"`).
- `tools.Registry.Add` appends tools mid-run without disturbing the earlier prefix; the agent loop
  re-reads `Agent.ToolDefs` each turn.
- Routes in `internal/routes/mcp.go` under `/api/mcp/*` (servers CRUD, connect, disconnect, reset-auth).
- Tests: `apps/server-go/tests/mcp/`.

Known gaps: no desktop UI, no `probe` endpoint, `loadTools` is only offered when at least one
enabled server exists at run start, and the system prompt does not yet mention MCP groups.

---

## 1. Architectural Principles

### 1.1 SDK choice
- Use `github.com/modelcontextprotocol/go-sdk` if it has reached a stable (or
  near-stable, feature-complete) tagged release when Phase 1 starts — official,
  co-designed with the spec, best long-term bet. Confirm it has streamable-HTTP
  client transport (required for Atlassian) before committing to it.
- Fall back to `github.com/mark3labs/mcp-go` if the official SDK is missing needed
  client features (streamable-HTTP, or OAuth helpers) at that time.
- Either way, isolate the chosen SDK behind our own package (`internal/agent/mcp`) so
  swapping libraries later doesn't touch the tool registry or agent loop.

### 1.2 Remote tool as a native `Tool`
- `internal/agent/tools.Tool` is already just an interface (`Name/Description/Tier/Schema/Execute`).
- An `mcpToolAdapter` wraps one remote MCP tool: `Schema()` returns the JSON schema the
  MCP server advertised directly (no invopop reflection needed — MCP tools already
  hand back raw JSON Schema), and `Execute` performs a `tools/call` RPC over the
  server's session instead of running local Go code.
- Adapters are **not** eagerly registered into the base `tools.Registry` — see §1.4
  (lazy loading). They're constructed on demand once a server's tool group is loaded.

### 1.3 Tiering
- MCP does not give us a read/write/exec classification. Default every remote tool to
  `TierWrite` (safer default, always prompts under `always-ask`) unless the server
  config overrides a tool's tier explicitly. Document this in the config schema.
- Atlassian's own permission groups (`read_jira`, `write_jira`, `delete_jira`, ...) are
  a natural hint: tool names/groups prefixed `read_`/`search_` can default to
  `TierRead`, `write_`/`manage_` to `TierWrite`, `delete_` to `TierExec`. Treat this as
  a per-server naming convention we can special-case, not something the protocol
  guarantees generically.

### 1.4 On-demand tool loading (ties into `harness-token-efficiency.md` Task 4.4)
- MCP tool schemas are not sent to the model by default. Each connected server is one
  **lazy-load group** (`mcp:<serverId>`), same mechanism as the `loadTools` tool
  planned for `webSearch`/`webFetch`/etc.
- The setup message gets one line per connected server: name + tool count (e.g.
  `mcp:atlassian (discoverable, connect required)` before auth, or
  `mcp:atlassian (12 tools)` once connected and listed).
- Calling `loadTools({group: "mcp:atlassian"})` triggers (if not already done)
  `tools/list` against that server and appends the resulting adapters to the tool
  list for the rest of the run, same append-only rule as other lazy groups (keeps the
  cached tool-definition prefix stable).
- This mirrors what Atlassian's server already does server-side (small "primary" tool
  set, `tools=all` override only for gateways that want everything up front) — we
  should *not* pass `tools=all`; let their own discovery narrow things further once
  our side has decided to load that server's group at all.

### 1.5 Config surface & storage — **not** `console.toml`
- MCP servers are user-level tooling (available across every project), not
  project-specific like `[scripts.*]`, and they carry secrets that shouldn't live in a
  file that might get committed. Store them the same way custom LLM provider
  endpoints are stored (`apps/server/providers/src/custom/endpoint-store.ts`'s
  pattern), not in `console.toml`.
- New file: `<ConsoleStorageDir>/mcp-servers.json` (i.e. `~/.console/mcp-servers.json`
  in prod, `~/.console-dev/mcp-servers.json` in dev — reuse
  `internal/utils.ConsoleStorageDir()`, don't invent a new path helper).
- One JSON file, one entry per server, not one-file-per-server — trivial full-read on
  daemon start, and CRUD mirrors `endpoint-store.ts`'s `listServers/getServer/saveServer/deleteServer`.
- Schema (draft):
  ```json
  {
    "servers": {
      "atlassian": {
        "label": "Atlassian",
        "transport": "http",
        "url": "https://mcp.atlassian.com/v2/mcp",
        "auth": { "type": "oauth2", "clientId": "...", "tokenRef": "atlassian" },
        "tierOverrides": { "delete_jira": "exec" },
        "enabled": true,
        "createdAt": 0,
        "updatedAt": 0
      },
      "local-fs": {
        "label": "Filesystem",
        "transport": "stdio",
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path"],
        "env": { "FOO": "bar" },
        "enabled": true
      }
    }
  }
  ```
- `id` follows the same `^[A-Za-z0-9_-]+$` rule used for project script ids.
- **Secrets live separately.** OAuth tokens and static API keys go in
  `~/.console/mcp-credentials.json` (0600, same as `SaveCredentialFile` in
  `providers/shared/oauth.go`), keyed by `tokenRef`. `mcp-servers.json` never holds a
  raw secret — only a reference — so it's safe to display/export without masking
  logic scattered everywhere. API reads of server config mask nothing because there's
  nothing sensitive left in that file.

### 1.6 Authentication — three shapes to support
1. **stdio env auth**: token/key passed as `env` to the subprocess. No network flow;
   the subprocess owns it. (Simple, ship in Phase 1 alongside stdio transport.)
2. **Static header / API token** (remote servers): a fixed `Authorization` header —
   either `Bearer <token>` or `Basic <base64(email:token)>` (Atlassian supports both:
   personal API token via Basic, service-account key via Bearer). User pastes the
   value once, we store it via `mcp-credentials.json` and attach the header on every
   request. No refresh logic needed. Ship this before OAuth — it's Atlassian's
   documented headless fallback and unblocks testing without a browser flow.
3. **OAuth 2.1** (remote servers, Atlassian's primary/recommended path): per the MCP
   auth spec, this includes **dynamic client registration** (RFC 7591 — the client
   registers itself with the server's `/register` endpoint rather than using a
   pre-shared client id), PKCE, and a browser-based authorization redirect. We already
   have PKCE + JWT-payload helpers in `providers/shared/oauth.go` (used by
   codex/claude/antigravity login) — this reuses that shape:
   - Discover the auth server via the MCP server's `.well-known/oauth-authorization-server`
     (or `oauth-protected-resource`) metadata.
   - Dynamic client registration once per server, cache the resulting `client_id`.
   - PKCE authorization-code flow: open the browser to the authorization URL, run a
     local loopback HTTP listener for the redirect (same pattern likely already used
     by one of the existing OAuth logins — check `codex/login.go` /
     `antigravity/login.go` for the loopback server helper before writing a new one).
   - Store access + refresh token in `mcp-credentials.json`; refresh transparently
     when a call gets a 401, retry once.
   - Note from the Atlassian docs: v1→v2 migrants sometimes need to clear cached
     client ids/`.well-known` credentials — our token store needs a manual "reset
     auth for this server" path (CLI command or API endpoint), not just delete-the-file.

---

## 2. Technical Invariants & Gotchas

1. **Lifecycle**: stdio servers are subprocesses — must be spawned on daemon/session
   start, killed on shutdown, and restarted with backoff if they crash mid-session.
   Remote HTTP servers have no process lifecycle, just a session/token lifecycle.
2. **Namespacing**: two servers can expose tools with the same name. Register remote
   tools under a namespaced id, e.g. `mcp__<serverId>__<toolName>`, to avoid
   collisions with native tools and with each other.
3. **Schema pass-through**: MCP JSON Schema may use draft features
   `invopop/jsonschema` doesn't emit (e.g. `oneOf`) — the adapter must pass the schema
   through unmodified rather than round-tripping it through Go structs.
4. **Timeouts & cancellation**: remote calls must respect the agent loop's per-call
   context cancellation (user abort) — don't let a hung MCP server stall a turn
   indefinitely; enforce a per-call timeout with a clear `ToolError` on expiry.
5. **Partial connectivity**: if one configured server fails to connect (or its OAuth
   token has expired and refresh fails) at startup, log and skip it — don't fail the
   whole tool registry build. Surface a "needs re-auth" state distinctly from "down".
6. **Result content types**: MCP tool results can be text, image, or embedded resource
   blocks; only text (and maybe image) needs to map into our `ToolResult.Content`
   shape initially — anything else degrades to a text placeholder for now.
7. **OAuth redirect UX in a headless/daemon context**: console's agent runs as a
   daemon, not always with a live desktop browser session next to it. The
   loopback-redirect flow needs a clear "open this URL to authorize" hand-off to
   the desktop app/CLI, not an assumption that the daemon can pop a browser itself.

---

## 3. Phased Implementation Plan

### Phase 1: Config & storage
**Goal**: Server configs and credentials have a home before any networking code exists.

- `internal/services/mcp_config.go`: CRUD over `~/.console/mcp-servers.json`
  (list/get/save/delete), id validation, following `endpoint-store.ts`'s shape.
- `internal/services/mcp_credentials.go`: CRUD over `~/.console/mcp-credentials.json`
  (0600), keyed by `tokenRef`. Static header creds and OAuth token pairs both live
  here, tagged by kind.
- **Verification**: `apps/server-go/tests/mcp/config_test.go`,
  `apps/server-go/tests/mcp/credentials_test.go` — CRUD, id validation, file perms,
  persistence across re-instantiation.

---

### Phase 2: Remote HTTP transport + static auth
**Goal**: Connect to Atlassian's server with a pasted API token before touching OAuth.

- Add chosen SDK to `go.mod` (per §1.1 — confirm streamable-HTTP client support).
- `internal/agent/mcp/client.go`:
  - `Connect(ctx, config ServerConfig, auth AuthProvider) (*Client, error)` for the
    `http` transport: dials `url`, attaches the `Authorization` header from
    `AuthProvider.Header()`, performs `initialize`, `tools/list`.
  - `Client.CallTool(ctx, name string, args json.RawMessage) (*CallResult, error)`.
  - `Client.Close() error`.
- `internal/agent/mcp/auth_static.go`: `AuthProvider` that returns a fixed
  `Bearer <token>` or `Basic <base64>` header from stored credentials.
- **Verification**: `apps/server-go/tests/mcp/client_test.go` — connect to a fixture
  HTTP MCP server (local test server implementing the protocol) with a static bearer
  token, assert `tools/list` + `tools/call` round-trip and that a bad token surfaces
  as a clear connection error.

---

### Phase 3: OAuth 2.1 (dynamic client registration + PKCE)
**Goal**: Support Atlassian's recommended auth path, not just the headless fallback.

- `internal/agent/mcp/auth_oauth.go`:
  - Discover authorization server metadata (`.well-known/oauth-authorization-server`
    or `oauth-protected-resource`, per MCP auth spec).
  - Dynamic client registration (RFC 7591) against the discovered `/register`
    endpoint; cache `client_id` in `mcp-credentials.json`.
  - PKCE authorize + token exchange, reusing `providers/shared.GeneratePKCE` /
    `PostTokenForm` / `TokenError`.
  - Loopback redirect handler — check whether `codex/login.go` or
    `antigravity/login.go` already has a reusable local-listener helper before
    writing a new one.
  - Refresh-token flow: on a `401` from `CallTool`/`tools/list`, refresh once and
    retry; if refresh fails, mark the server "needs re-auth" (§2.5) instead of
    hard-failing.
  - Manual "reset auth for this server" operation (clears cached `client_id` +
    tokens) — needed for the v1→v2-style migration gotcha Atlassian's docs call out.
- **Where the "open this URL" hand-off goes**: an internal event
  (`EventMCPAuthRequired{ServerID, URL}`) the daemon emits, surfaced by
  the CLI/desktop the same way other user-facing prompts are (check how
  `codex`/`claude` OAuth login already surfaces its authorize URL to the caller —
  reuse that path rather than inventing a second one).
- **Verification**: `apps/server-go/tests/mcp/oauth_test.go` — fake authorization
  server, assert dynamic registration, PKCE exchange, refresh-on-401, and reset-auth
  clearing stored state.

---

### Phase 4: stdio transport
**Goal**: Support local subprocess MCP servers (filesystem, git, etc.) alongside remote ones.

- Extend `internal/agent/mcp/client.go` with the `stdio` transport: spawn
  `command`/`args`/`env`, wire stdio pipes to the SDK's transport, same
  `initialize`/`tools/list`/`tools/call` surface as Phase 2.
- `internal/agent/mcp/manager.go`:
  - `NewManager(configs []ServerConfig) *Manager`
  - `Manager.Start(ctx) error` — connects to all enabled servers concurrently
    (mixed stdio + http), tolerates individual failures (log + skip, per §2.5).
  - `Manager.Stop()` — closes all clients, kills subprocesses.
  - `Manager.Groups() []ToolGroup` — one group per server, undiscovered until loaded
    (§1.4), not a flat eager tool list.
- **Verification**: `apps/server-go/tests/mcp/manager_test.go` — partial-failure
  tolerance across mixed transports, shutdown kills subprocesses, groups stay
  undiscovered until explicitly loaded.

---

### Phase 5: Tool adapter & lazy-load integration
**Goal**: Remote tools show up to the model exactly like native tools, only when asked for.

- `internal/agent/mcp/adapter.go`:
  - `NewAdapter(client *Client, serverID string, remoteTool RemoteTool, tier tools.ToolTier) tools.Tool`
  - `Name()` returns the namespaced id (§2.2); `Schema()` passes through the raw
    MCP schema; `Execute` calls `client.CallTool` with a per-call timeout and maps
    the result/error into `tools.ToolResult` / `*tools.ToolError`.
  - Tier resolution: config `tierOverrides` first, then the naming-convention
    heuristic (§1.3), then default `TierWrite`.
- Wire `Manager.Groups()` into the `loadTools`/lazy-tool mechanism from
  `harness-token-efficiency.md` Task 4.4 — each MCP server is a `mcp:<serverId>`
  group; loading it calls `tools/list` (if not already cached) and appends adapters
  to the run's tool list.
- **Verification**: `apps/server-go/tests/mcp/adapter_test.go` — fake client, assert
  schema pass-through, tier resolution order, timeout produces `ToolError`;
  `tests/agent/loadtools_test.go` (or wherever Task 4.4's tests land) extended to
  cover an `mcp:*` group loading mid-run and appearing only after `loadTools`.

---

### Phase 6: UX & config surface (deferred until core is stable)
**Goal**: Let users add/remove MCP servers and go through OAuth without hand-editing JSON.

- API endpoints mirroring the custom-provider pattern: `GET/POST/DELETE
  /api/mcp-servers`, `POST /api/mcp-servers/:id/probe`, `POST
  /api/mcp-servers/:id/authorize` (kicks off Phase 3's OAuth flow and returns the
  URL to open), `POST /api/mcp-servers/:id/reset-auth`.
- Desktop UI: list of configured servers with connection/auth status ("connected" /
  "needs re-auth" / "down"), add/edit dialog, per-tool tier override editor,
  "Authorize" button that opens the system browser and polls for completion.
- Not required for the harness itself to use MCP tools — config-file-only (with a
  CLI helper to paste tokens / trigger OAuth) is enough for Phases 1–5.

---

## 4. Verification & Testing Matrix

| Test Case | Method | Expected Outcome |
| :--- | :--- | :--- |
| **Config/credential CRUD** | `go test ./tests/mcp/... -run TestConfig` | Servers and credentials persist correctly, invalid ids rejected, credential file is 0600 |
| **HTTP round-trip (static auth)** | `go test ./tests/mcp/... -run TestClient` | `initialize`, `tools/list`, `tools/call` succeed against a fixture HTTP server with a bearer/basic token |
| **OAuth dynamic registration + PKCE** | `go test ./tests/mcp/... -run TestOAuth` | Registration, authorize, token exchange, and refresh-on-401 all succeed against a fake auth server |
| **Reset auth** | `go test ./tests/mcp/... -run TestOAuth` | Clears cached client id + tokens, forces a fresh registration+authorize on next connect |
| **stdio round-trip** | `go test ./tests/mcp/... -run TestClient` | Same protocol surface as HTTP, against a spawned fixture process |
| **Partial failure tolerance** | `go test ./tests/mcp/... -run TestManager` | One bad server (stdio crash or expired OAuth token) doesn't block the others from connecting |
| **Lazy loading** | `go test ./tests/agent/... -run TestLoadTools` | `mcp:<serverId>` group tools are absent from the tool list until `loadTools` is called, then appended without disturbing earlier prefix |
| **Adapter schema pass-through** | `go test ./tests/mcp/... -run TestAdapter` | Remote JSON Schema reaches `Definitions()` unmodified |
| **Tier resolution** | `go test ./tests/mcp/... -run TestAdapter` | Config override wins, then naming heuristic (`read_*`→read, `delete_*`→exec), then `TierWrite` default |
| **Call timeout** | `go test ./tests/mcp/... -run TestAdapter` | Hung remote call returns `ToolError` within the configured timeout, doesn't stall the turn |
