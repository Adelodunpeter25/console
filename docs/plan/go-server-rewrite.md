# Plan: Rewrite `apps/server` in Go (`apps/server-go`)

Status: **in progress** — Phases 0–1 complete, Phase 2 (agent core) started
(tool framework, permissions, event stream, initial loop), Phase 4/5 partially complete
(session storage, fs, git, scripts, ports, favorites, projects, terminal
WebSocket, fff integration). Remaining: rest of agent core (Phase 2),
providers (Phase 3), remaining Phase 5 services, parity harness, OpenAPI
contract, cutover, CLI rewrite.

Goal: replace the Bun/TypeScript server (`apps/server`, ~39k LOC across
`agent/`, `api/`, `providers/`) with a Go implementation at `apps/server-go`,
preserving the HTTP/SSE/Socket.IO wire contract exactly — same routes, same
request and response shapes — so no client has to change. Clients are
different languages and there is no shared type package; instead the Go
server generates an OpenAPI spec from its types, and the spec must match
the current API 1:1.

Ground rules:

- The rewrite runs **alongside** the current server until it reaches parity.
  The TS server stays the source of truth until Phase 8.
- HTTP framework: **Fiber** (fasthttp-based), chosen for performance in the
  terminal and API hot paths.
- The **cline provider is not ported** — it is unused and dropped in the Go
  server (including its `/cline` route).
- The **console CLI (`apps/cli`) is rewritten in Go** as a compiled binary
  (Phase 9).
- File search keeps using the same `@ff-labs/fff-node`-style finder approach;
  the Go side implements an equivalent fast finder (see Phase 5).
- No API contract changes ride along with the rewrite. Parity first,
  improvements after.

---

## Phase 0 — Foundations

- [x] Scaffold `apps/server-go` module (`go.mod`, `cmd/server`, `internal/`).
- [x] Set up Fiber app skeleton: routes registration matching current paths,
      middleware, error handler.
- [x] Define package layout mirroring existing seams (so far
      `internal/types/` (one file per type), `internal/db/` (database
      manager + schema only), `internal/routes/`, `internal/services/`,
      `internal/utils/`, `tests/`; the rest arrive with their phases):
      `internal/agent/`, `internal/api/`, `internal/providers/<name>/`,
      `internal/session/`, `internal/tools/`, `internal/types/`.
- [ ] Decide config loading (BurntSushi/toml vs pelletier/go-toml/v2) and
      structured logging (log/slog).
- [ ] Pick OpenAPI generation approach from Fiber handlers/types
      (candidates: swaggo/swag, huma, or hand-maintained spec generated in
      CI). The spec must reproduce the current request/response shapes
      exactly so existing clients keep working. **Blocker for Phase 8** —
      agree the shape early.

## Phase 1 — Storage layer (`agent/src/session`, `agent/src/memory`)

TS deps to replace: `bun:sqlite` (9 files), `zod` schemas.

- [x] Port session schema as a single canonical DDL — **no migration
      compatibility**, no ALTER TABLE logic; schema changes recreate
      storage (source: agent/src/session/schema.ts).
- [x] Go toolchain: 1.25. SQLite driver: `modernc.org/sqlite` (pure Go, no cgo).
- [x] Port session storage/ops (initial slice): storage.ts, session-ops.ts,
      session-messages.ts — create, list, load with cursor pagination,
      append/replace messages, soft delete. Remaining: repair, orphan-file
      self-heal, subagents, todos, queued prompt, file changes.
- [x] Port projects.ts and model-favorites.ts.
- [ ] Port memory subsystem: schema.ts, storage.ts, search.ts, registry.ts.
- [x] Add a DB access policy (single writer mutex + WAL) replacing Bun's
      implicit single-threaded sync behavior.
- [x] Tests (initial slice): project CRUD, session lifecycle (create /
      append / dedupe / load / soft delete), model favorites, project
      deletion cascade — `apps/server-go/internal/session/storage_test.go`.
      tests/memory and remaining ops pending.

## Phase 2 — Agent core (`agent/src/service`, `agent/src/types`)

- [x] Tool framework: generic `NewTool[I]` deriving JSON Schema from struct
      tags via `invopop/jsonschema`; decode-to-struct doubles as validation.
      Tools so far: read_file, write_file, list_dir, glob, grep, editFile,
      batchWrite, readSkill, fetch, webSearch, ask, askMany, todo,
      bash, bashJob.
      glob/grep are fff-powered (native fff_glob/fff_live_grep via the same
      CGo bindings backing /api/fs/search), falling back to a filesystem
      walk when fff is unavailable.
      editFile does exact-string find/replace (errors on 0 or >1 matches);
      batchWrite writes multiple files concurrently (or stop-on-error);
      readSkill loads full skill content on demand via the new
      `internal/agent/systemprompt` skill discovery.
      fetch tries keyless Firecrawl markdown extraction for GET requests to
      likely web pages (`internal/agent/tools/firecrawl.go`, live-verified
      against a real URL), falling back to a direct HTTP request with
      content-type-aware formatting (JSON pretty-print, naive HTML-to-text)
      for everything else and any Firecrawl miss.
      webSearch tries keyless Firecrawl search first (full markdown per
      result, live-verified against a real query), falls back to scraping
      DuckDuckGo's HTML lite page on a retryable Firecrawl error or empty
      result set, or calls Brave's API directly when `searchEngine: brave`
      is requested (needs `BRAVE_SEARCH_API_KEY`).
      ask/askMany take an optional `AskHandler`; the `Ask`/`AskMany`
      singletons in `DefaultTools()` run headless (auto-pick the first
      option or report "skipped"), while `NewAskTool`/`NewAskManyTool`
      build interactive instances once a real handler (approval-channel
      wired) exists — same pattern as the executor's `Approver`.
      todo is genuinely persisted (not memory-only) — `session_todos` table
      per session, matching TS's session-todos.ts exactly: `NewTodoTool`
      lazy-loads existing items from `SessionService.GetSessionTodos` on
      first use and delete-then-insert persists after every mutation via
      `SaveSessionTodos`; a fresh tool instance for the same session id
      picks up prior state (verified with two independent tool instances
      against one real SQLite session). Added the matching
      `GET /api/sessions/:id/todos` route (live-curled against a running
      server) and `ClearSessionTodos`. The `Todo` singleton in
      `DefaultTools()` is unbound (nil store) for the process lifetime,
      matching TS's default `createTodoTool()` export used for offline/
      static registration; a real run binds its own via `NewTodoTool`.
      Also ported `ClearCompletedTodos` (`SessionService`): matches
      RunService's end-of-run cleanup in the TS server (`finally` block of
      `runAgentStream`) — once every item in a non-empty list is
      "completed", the whole list is wiped from `session_todos` so the
      next run starts fresh; a partially-done list is left untouched. This
      is a run-orchestration concern, not the tool's — the tool only ever
      marks status, so the primitive is on `SessionService` ready for the
      future run layer (Phase 3+) to call after each run settles.
      bash/bashJob: `internal/services/bash_capture.go` (sync mode, port
      of `spawnCapture` — captures stdout/stderr with a byte cap while
      draining past it so the child still exits normally, tree-kills via
      process group on timeout/context-cancel) and
      `internal/services/bash_job_manager.go` (background mode, port of
      `manager.ts` — one job per detached process group, ring-buffered
      output per stream, cursor-paginated reads, retention after
      completion, per-session ownership). `NewBashTool`/`NewBashJobTool`
      share one `*BashJobManager` per run (bash starts jobs, bashJob polls
      them); the `Bash`/`BashJob` singletons in `DefaultTools()` have no
      manager (background mode errors cleanly). Verified live: real
      subprocess exit codes/output capture, context-cancellation abort,
      background-job timeout/expire, and process-group kill all exercised
      against actual child processes (not mocked).
      Still to port: memory (own persistent store); subagent waits on
      Phase 3 providers since it runs a nested agent-loop turn against a
      real model.
- [x] Port permissions (approval.ts) as tier/mode → policy resolution
      (`internal/agent/permissions`); plan mode hard-denies write/exec.
- [x] Generic queue-based event stream (`internal/agent/stream`),
      loss-free under producer/consumer speed mismatch.
- [ ] Port agent-loop.ts, stream-turn.ts, tool-executor.ts, tool-input.ts
      (initial slice done: sequential turns, tool-call/result cycle, session
      persistence, mock-provider round-trip test; still missing compaction,
      subagents, todos, queued prompts, thinking-block validation).
- [ ] Port model-roles.ts, role-resolver.ts, thinking.ts,
      validate-thinking.ts, session-title.ts.
- [ ] Port event-stream.ts (SSE) using `http.Flusher`.
- [ ] Port compaction (cut-point, file-tracker, llm-compaction, shake,
      structural-summary, token-estimator).
- [x] Port system prompts and discovery (`internal/agent/systemprompt`):
      walk-up config-dir helpers, frontmatter parsing, skills/rules/commands
      discovery, AGENTS.md/CLAUDE.md context files, SYSTEM.md override,
      workspace tree, environment info, approval-mode instructions, and the
      `BuildSystemPrompt` assembler — full parity with
      apps/server/agent/src/systemprompt/*.
- [x] Tests: tool schema generation, tool validation, permission matrix,
      mock-provider loop round-trip (tool call + persistence), stream
      no-loss, plan-mode denial, glob/grep fff + fallback, editFile,
      batchWrite, readSkill, fetch (direct JSON/HTML/POST/error-status via
      httptest, plus a live Firecrawl smoke test against a real URL),
      webSearch (Firecrawl success, retryable-error fallback to DuckDuckGo,
      Brave success/missing-key, all via local httptest doubles, plus a
      live smoke test against a real query),
      ask/askMany (headless default-option fallback, handler-driven
      answers, shared batch id across askMany's questions),
      todo (unbound in-memory lifecycle, and persistence across two
      independent tool instances backed by a real SQLite session,
      plus ClearCompletedTodos: partial lists survive, fully-completed
      lists are wiped, empty lists are a no-op),
      bash/bashJob (sync success/non-zero-exit/timeout/missing-command,
      background start->wait->list lifecycle, cross-session ownership
      isolation, kill, missing-jobId/unknown-action errors — all against
      real subprocesses),
      system-prompt discovery + assembly, approval-mode instructions
      (`tests/agent_test.go`, `tests/tools_more_test.go`,
      `tests/fetch_tool_test.go`, `tests/web_search_tool_test.go`,
      `tests/ask_tools_test.go`, `tests/todo_tool_test.go`,
      `tests/bash_tools_test.go`, `tests/systemprompt_test.go`).

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
- [x] fff integration: CGo bindings to the fff C ABI
      (github.com/dmtrKovalenko/fff, crates/fff-c) — prebuilt shared lib
      loaded at runtime via dlopen (`third_party/fff/libfff_c.so` or
      `FFF_LIB_PATH`), per-root indexed instances with background watcher,
      wired into /api/fs/search with a walk-based fallback until the index
      is warm. The binary is not committed — fetch per machine with
      `apps/server-go/scripts/fetch-fff-lib.sh` (Linux x64/arm64, macOS
      x64/arm64) or set `FFF_LIB_PATH`.
- [ ] Tests: port tests/tools.

## Phase 5 — API surface (`api/src/`)

TS deps to replace: `hono`, `hono/streaming`, `zod`, `serve-sim`.
Performance goal: Fiber/fasthttp on the hot paths — API endpoints and the
terminal socket.

- [ ] Fiber router skeleton, middleware, error handling; route registration
      matching paths 1:1 with apps/server/api/src/routes (17 route files
      after dropping cline: assist, auth, config, devices, fs, git,
      model-favorites, notifications, ports, project-scripts, projects,
      providers, run, sessions, settings, usage).
- [x] Port services (done: session, project, model-favorites, fs,
      fswatch (fsnotify recursive), git (os/exec git), project-scripts
      (console.toml parse + managed runs + SSE stream + process-group stop),
      port-registry (output observation, liveness reaper, proxy ports
      45000+ with HTTP/WS passthrough) + port-tunnel WebSocket, settings
      (model roles), usage (wire shape only — reports stay null until the
      Phase 3 quota fetchers), notification bus + SSE stream, config
      approval-modes, assist (skills discovery + fff-backed @-mention
      search). Remaining: provider, auth (needs Phase 3 OAuth/token-store
      internals), run (session-level). Devices intentionally skipped.
- [x] Port terminal (initial): pty.manager (`creack/pty`) + `/api/terminals`
      WebSocket via `fasthttp/websocket` — JSON protocol {spawned, output,
      exit, error} / {input, resize, kill} and ?proto=binary tag framing;
      verified end to end. Remaining vs TS: send-buffer backpressure pause.
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

## Phase 7 — OpenAPI contract

- [ ] Generate the OpenAPI document from the Go server (build tag or CI
      step) and diff it against a spec captured from the TS server before
      cutover. Gate: no breaking differences in any request/response shape.
- [ ] Publish the spec as a repo artifact so all language clients can
      reference or codegen from one source. No client changes required at
      cutover.

## Phase 9 — Console CLI rewrite in Go

- [ ] Port apps/cli (~900 LOC: commander-based bin, index.ts, types.ts)
      to Go, compiled to a single static binary.
- [ ] CLI framework: cobra or urfave/cli; source commands from the same
      OpenAPI spec produced in Phase 7.
- [ ] Keep the `console` bin name and existing command/flag surface.
- [ ] Ship release binaries (goreleaser) and update install.sh.

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
2. **Contract drift** — with no shared type package, the OpenAPI spec
   (Phase 7) is the only guard that clients keep working; diff it against
   the TS server before cutover.
3. **SSE/socket protocol drift** — clients must not detect the swap;
   Phase 6 diffing is the gate.
4. **Bun-specific assumptions** — single-threaded sqlite access, Bun APIs
   in scripts; call these out during each port.
