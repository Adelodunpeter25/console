# Plan: Rewrite `apps/server` in Go (`apps/server-go`)

Status: **planned — not started**

Goal: replace the Bun/TypeScript server (`apps/server`, ~39k LOC across
`agent/`, `api/`, `providers/`) with a Go implementation at `apps/server-go`,
preserving the HTTP/SSE/Socket.IO wire contract so the existing TS clients
(`apps/desktop`, `apps/cli`, `packages/api`) keep working.

Ground rules:

- The rewrite runs **alongside** the current server until it reaches parity.
  The TS server stays the source of truth until Phase 8.
- File search keeps using the same `@ff-labs/fff-node`-style finder approach;
  the Go side implements an equivalent fast finder (see Phase 5).
- No API contract changes ride along with the rewrite. Parity first,
  improvements after.

---

## Phase 0 — Foundations

- [ ] Scaffold `apps/server-go` module (`go.mod`, `cmd/server`, `internal/`).
- [ ] Pick HTTP stack: Go 1.22+ std `net/http` (recommended) or `chi` for
      middleware ergonomics.
- [ ] Define package layout mirroring existing seams:
      `internal/agent/`, `internal/api/`, `internal/providers/<name>/`,
      `internal/session/`, `internal/tools/`, `internal/types/`.
- [ ] Decide config loading (BurntSushi/toml vs pelletier/go-toml/v2) and
      structured logging (log/slog).
- [ ] Decide contract artifact for TS clients: OpenAPI or JSON Schema
      generated from Go types, with codegen into `packages/api` /
      `@console/types`. **Blocker for Phase 8** — agree the shape early.

## Phase 1 — Storage layer (`agent/src/session`, `agent/src/memory`)

TS deps to replace: `bun:sqlite` (9 files), `zod` schemas.

- [ ] Port session schema + migrations (agent/src/session/schema.ts,
      session-changes.ts).
- [ ] Choose SQLite driver: `modernc.org/sqlite` (pure Go, no cgo,
      recommended) or `mattn/go-sqlite3`.
- [ ] Port session storage/ops: storage.ts, session-ops.ts,
      session-messages.ts, session-helpers.ts, utils.ts.
- [ ] Port projects.ts and model-favorites.ts.
- [ ] Port memory subsystem: schema.ts, storage.ts, search.ts, registry.ts.
- [ ] Add a DB access policy (single writer mutex + WAL) replacing Bun's
      implicit single-threaded sync behavior.
- [ ] Tests: port apps/server/tests/sessions and tests/memory to Go table
      tests against the same schema.

## Phase 2 — Agent core (`agent/src/service`, `agent/src/types`)

- [ ] Port agent-loop.ts, stream-turn.ts, tool-executor.ts, tool-input.ts.
- [ ] Port model-roles.ts, role-resolver.ts, thinking.ts,
      validate-thinking.ts, session-title.ts.
- [ ] Port event-stream.ts (SSE) using `http.Flusher`.
- [ ] Port permissions (approval.ts) and compaction
      (cut-point, file-tracker, llm-compaction, shake, structural-summary,
      token-estimator).
- [ ] Port system prompts and types (types/index, types/system-prompt).
- [ ] Tests: port tests/agent.

## Phase 3 — Provider layer (`providers/src/`) — highest risk

TS deps to replace: `ai`, `@ai-sdk/openai`, `@ai-sdk/openai-compatible`,
`@ai-sdk/provider-utils`. The Vercel AI SDK hides stream-delta assembly,
tool-call chunking, thinking blocks, and per-provider quirks; this is where
~80% of the porting effort lives.

- [ ] Build a shared streaming core: SSE client, message conversion,
      tool-call delta assembly, usage accounting (replaces
      shared/openai-compat-*.ts).
- [ ] Port claude provider: constants, convert, oauth, stream-fn, discovery.
      Use `anthropics/anthropic-sdk-go` where it fits.
- [ ] Port codex provider: constants, oauth, stream-fn.
- [ ] Port cline provider (stream-fn uses the `ai` SDK directly).
- [ ] Port remaining providers: antigravity, devin.
- [ ] Port discovery/fetch-models and provider auth token-store.
- [ ] Port usage tracking (providers/src/usage).
- [ ] Tests: port tests/providers; record live-request fixtures for
      regression before deleting the TS versions.

## Phase 4 — Tools (`agent/src/tools/`)

TS deps to replace: `@ff-labs/fff-node` (FileFinder), `diff`,
`zod`-declared tool schemas.

- [ ] Define tool interface + JSON Schema authoring pattern (replaces zod +
      zod-to-json-schema; candidate: invopop/jsonschema or literal schema
      structs).
- [ ] Port file tools: read (engine), write-file, edit-file, batch-write,
      list-dir, fs-common.
- [ ] Implement Go file finder for glob.ts/grep.ts matching fff-node
      semantics (filepath.WalkDir + doublestar; parallel walk).
- [ ] Port bash tool suite: bash.ts, manager.ts, proc.ts, job-tool.ts
      (process supervision via os/exec + context).
- [ ] Port remaining tools: fetch, firecrawl, web-search, todo, memory,
      subagent, read-skill, ask.
- [ ] Port fff-bootstrap.ts.
- [ ] Tests: port tests/tools.

## Phase 5 — API surface (`api/src/`)

TS deps to replace: `hono`, `hono/streaming`, `zod`, `serve-sim`.

- [ ] Router skeleton, middleware, error handling; route registration
      matching paths 1:1 with apps/server/api/src/routes (18 route files:
      assist, auth, cline, config, devices, fs, git, model-favorites,
      notifications, ports, project-scripts, projects, providers, run,
      sessions, settings, usage).
- [ ] Port services: session, project, provider, git (os/exec git),
      fs, fswatch (fsnotify recursive), notification, usage, auth,
      port-registry, port-tunnel socket, assist, project-scripts, run.
- [ ] Port terminal: pty.manager + socket.route (`creack/pty`; keep the
      socket protocol byte-identical).
- [ ] Port device managers (device.manager, ios.manager) — audit what
      `agent-device` wraps first; likely shell out to adb/xcrun.
- [ ] Tests: port tests/api + tests/terminal; contract tests diffing
      responses against the TS server on a fixture project.

## Phase 6 — Parity harness

- [ ] Run TS and Go servers side by side; script identical requests through
      both (sessions CRUD, agent turn, fs ops, terminal, providers list).
- [ ] Diff SSE event streams field-by-field for one full agent turn per
      provider.
- [ ] Wire `apps/cli` / `apps/desktop` against the Go server in a dev
      environment and exercise a full agent session.
- [ ] Performance smoke: startup time, memory, concurrent sessions.

## Phase 7 — Client type contract

- [ ] Generate OpenAPI/JSON Schema from Go types; publish into
      packages/api / @console/types (codegen or CI-checked artifacts).
- [ ] Keep TS types compiling unchanged for desktop/cli/android.

## Phase 8 — Cutover

- [ ] Flip dev scripts (`apps/server` package.json scripts, Makefile,
      console.toml) to the Go binary.
- [ ] Archive TS server code (keep tests as reference) or delete after a
      soak period — decide then.
- [ ] Update docs, AGENTS.md test commands, and CI to the Go test runner.

---

## Key dependency map

| TS | Go |
|---|---|
| `ai`, `@ai-sdk/*` | anthropic-sdk-go / openai-go + hand-rolled shared streaming core |
| `bun:sqlite` | modernc.org/sqlite (pure Go) |
| `hono`, `hono/streaming` | std net/http (+ chi if needed) |
| `zod`, `zod-to-json-schema` | invopop/jsonschema or literal JSON Schema structs |
| `@ff-labs/fff-node` | native Go finder (WalkDir + doublestar), same semantics |
| `diff` | sergi/go-diff or similar |
| `@iarna/toml` | BurntSushi/toml |
| `agent-device` | adb/xcrun subprocesses (audit first) |
| node `fs.watch` | fsnotify |
| node-pty equivalent | creack/pty |
| `@console/types`, `@console/utils` | internal packages + generated client types |

## Risks

1. **Provider layer depth** — the AI SDK does invisible protocol work;
   budget most of the effort here (Phase 3).
2. **Type sharing** — TS clients lose the compiler contract unless Phase 7
   lands; do the codegen decision in Phase 0.
3. **SSE/socket protocol drift** — clients must not detect the swap;
   Phase 6 diffing is the gate.
4. **Bun-specific assumptions** — single-threaded sqlite access, Bun APIs
   in scripts; call these out during each port.
