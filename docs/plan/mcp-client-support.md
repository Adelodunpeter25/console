# MCP Client Support — Implementation Plan

Adds first-class **MCP client** capability to the agent: the harness connects out to
user-configured MCP servers (local stdio processes or remote HTTP/SSE endpoints) and
exposes their tools to the model alongside the existing native tools in
`apps/server-go/internal/agent/tools`. This is a client integration only — console
does not ship an MCP server.

Target layout: `apps/server-go/internal/agent/mcp/`

---

## 1. Architectural Principles

### 1.1 SDK choice
- Use `github.com/modelcontextprotocol/go-sdk` if it has reached a stable (or
  near-stable, feature-complete) tagged release by the time Phase 1 starts — official,
  co-designed with the spec, best long-term bet.
- Fall back to `github.com/mark3labs/mcp-go` if the official SDK is still missing
  needed client features (e.g. streamable-HTTP transport) at that time.
- Either way, isolate the chosen SDK behind our own package (`internal/agent/mcp`) so
  swapping libraries later doesn't touch the tool registry or agent loop.

### 1.2 Remote tool as a native `Tool`
- `internal/agent/tools.Tool` is already just an interface (`Name/Description/Tier/Schema/Execute`).
- An `mcpToolAdapter` wraps one remote MCP tool: `Schema()` returns the JSON schema the
  MCP server advertised directly (no invopop reflection needed — MCP tools already
  hand back raw JSON Schema), and `Execute` performs a `tools/call` RPC over the
  server's session instead of running local Go code.
- Adapters get registered into the existing `tools.Registry` next to native tools —
  the agent loop and provider-facing `Definitions()` do not need to know the
  difference.

### 1.3 Tiering
- MCP does not give us a read/write/exec classification. Default every remote tool to
  `TierWrite` (safer default, always prompts under `always-ask`) unless the server
  config overrides a tool's tier explicitly. Document this in the config schema.

### 1.4 Config surface
- Reuse the `console.toml`-style pattern already used for `[scripts.*]`
  (`internal/services/project_scripts_config.go`) for consistency:
  ```toml
  [mcp.<id>]
  label = "Filesystem"
  transport = "stdio"          # "stdio" | "sse" | "http"
  command = "npx"              # stdio only
  args = ["-y", "@modelcontextprotocol/server-filesystem", "/path"]
  env = { FOO = "bar" }        # stdio only
  url = "https://example.com/mcp"  # sse/http only
  headers = { Authorization = "Bearer ..." }
  tier_overrides = { write_file = "write", delete_file = "exec" }
  enabled = true
  ```
- `id` follows the same `^[A-Za-z0-9_-]+$` rule as project scripts.

---

## 2. Technical Invariants & Gotchas

1. **Lifecycle**: stdio servers are subprocesses — must be spawned on daemon/session
   start, killed on shutdown, and restarted with backoff if they crash mid-session.
2. **Namespacing**: two servers can expose tools with the same name (e.g. both ship a
   `search` tool). Register remote tools under a namespaced id, e.g. `mcp__<serverId>__<toolName>`,
   to avoid collisions with native tools and with each other.
3. **Schema pass-through**: MCP JSON Schema may use draft features `invopop/jsonschema`
   doesn't emit (e.g. `oneOf`) — the adapter must pass the schema through unmodified
   rather than round-tripping it through Go structs.
4. **Timeouts & cancellation**: remote calls must respect the agent loop's per-call
   context cancellation (user abort) — don't let a hung MCP server stall a turn
   indefinitely; enforce a per-call timeout with a clear `ToolError` on expiry.
5. **Partial connectivity**: if one configured server fails to connect at startup, log
   and skip it — don't fail the whole tool registry build.
6. **Result content types**: MCP tool results can be text, image, or embedded resource
   blocks; only text (and maybe image) needs to map into our `ToolResult.Content` shape
   initially — anything else degrades to a text placeholder for now.

---

## 3. Phased Implementation Plan

### Phase 1: SDK evaluation & connection primitive
**Goal**: Prove out one working stdio connection end-to-end before building the adapter layer.

- Pick SDK per §1.1; add to `go.mod`.
- `internal/agent/mcp/client.go`:
  - `Connect(ctx, config ServerConfig) (*Client, error)` — spawns/dials, performs
    MCP `initialize` handshake, lists tools (`tools/list`).
  - `Client.CallTool(ctx, name string, args json.RawMessage) (*CallResult, error)`.
  - `Client.Close() error`.
- Support stdio transport only in this phase.
- **Verification**: `apps/server-go/tests/mcp/client_test.go` — spawn a tiny fixture
  MCP server (or use a well-known npm one via `npx`) and assert `tools/list` +
  `tools/call` round-trip.

---

### Phase 2: Config parsing & server manager
**Goal**: Load `[mcp.*]` blocks from `console.toml`, manage connection lifecycle.

- `internal/services/mcp_config.go`: parse/validate `[mcp.*]` tables (mirrors
  `project_scripts_config.go`'s ordering + validation approach).
- `internal/agent/mcp/manager.go`:
  - `NewManager(configs []ServerConfig) *Manager`
  - `Manager.Start(ctx) error` — connects to all enabled servers concurrently,
    tolerates individual failures (log + skip, per §2.5).
  - `Manager.Stop()` — closes all clients, kills subprocesses.
  - `Manager.Tools() []RemoteTool` — flattened list across all connected servers.
- **Verification**: `apps/server-go/tests/mcp/config_test.go` (parsing/validation),
  `apps/server-go/tests/mcp/manager_test.go` (partial-failure tolerance, shutdown
  kills subprocesses).

---

### Phase 3: Tool adapter & registry integration
**Goal**: Remote tools show up to the model exactly like native tools.

- `internal/agent/mcp/adapter.go`:
  - `NewAdapter(client *Client, serverID string, remoteTool RemoteTool, tier tools.ToolTier) tools.Tool`
  - `Name()` returns the namespaced id (§2.2); `Schema()` passes through the raw
    MCP schema; `Execute` calls `client.CallTool` with a per-call timeout and maps
    the result/error into `tools.ToolResult` / `*tools.ToolError`.
- Wherever the native registry is built (agent session bootstrap), append
  `manager.Tools()` adapters via `tools.NewRegistry(append(nativeTools, mcpTools...)...)`.
- Apply `tier_overrides` from config, defaulting to `TierWrite`.
- **Verification**: `apps/server-go/tests/mcp/adapter_test.go` — fake client,
  assert schema pass-through, tier defaulting/override, timeout produces `ToolError`.

---

### Phase 4: HTTP/SSE transport + reconnect
**Goal**: Support remote (non-subprocess) MCP servers and recover from drops.

- Extend `internal/agent/mcp/client.go` with SSE/streamable-HTTP transport
  (`transport = "sse" | "http"` in config), including `headers` for auth.
- Add reconnect-with-backoff to `Manager` for both stdio (process crash) and
  remote (connection drop) cases; surface connection state so the UI can show a
  server as degraded/offline instead of silently dropping its tools.
- **Verification**: `apps/server-go/tests/mcp/reconnect_test.go` — simulate a
  server going down mid-session, assert tools become unavailable then
  reappear after reconnect.

---

### Phase 5: UX & config surface (deferred until core is stable)
**Goal**: Let users add/remove MCP servers without hand-editing `console.toml`.

- API endpoints mirroring the custom-provider pattern
  (`GET/POST/DELETE /api/mcp-servers`, `POST /api/mcp-servers/:id/probe`).
- Desktop UI: list of configured servers with connection status, add/edit dialog,
  per-tool tier override editor.
- Not required for the harness itself to start using MCP tools — config-file-only
  is enough for Phases 1–4.

---

## 4. Verification & Testing Matrix

| Test Case | Method | Expected Outcome |
| :--- | :--- | :--- |
| **Stdio round-trip** | `go test ./tests/mcp/... -run TestClient` | `initialize`, `tools/list`, `tools/call` succeed against a fixture server |
| **Config parsing** | `go test ./tests/mcp/... -run TestConfig` | Valid `[mcp.*]` blocks parse; invalid ids/transports rejected |
| **Partial failure tolerance** | `go test ./tests/mcp/... -run TestManager` | One bad server config doesn't block the others from connecting |
| **Adapter schema pass-through** | `go test ./tests/mcp/... -run TestAdapter` | Remote JSON Schema reaches `Definitions()` unmodified |
| **Tier defaulting/override** | `go test ./tests/mcp/... -run TestAdapter` | Untagged tools default to `TierWrite`; config override respected |
| **Call timeout** | `go test ./tests/mcp/... -run TestAdapter` | Hung remote call returns `ToolError` within the configured timeout, doesn't stall the turn |
| **Reconnect** | `go test ./tests/mcp/... -run TestReconnect` | Server drop removes its tools; recovery restores them without a full daemon restart |
