# Bun Migration Plan

This document describes the plan to fully migrate `apps/server` from Node.js (run via `tsx`) to the Bun runtime. It is a prerequisite for the single-binary / native compilation effort: **do not attempt native compilation (`bun build --compile`) until every step in this document is complete and verified.** Compiling early would freeze in Node-specific code paths and native addon loading that defeat the purpose of the binary.

## Why migrate to Bun

The end goal is a single self-contained binary distributed via a `curl | sh` installer that runs as a daemon, prints its port, and accepts connections from any client (mobile, desktop, CLI) anywhere.

Bun makes this possible because it replaces both of the server's native dependencies with runtime builtins:

| Dependency | Today (Node + tsx) | After migration |
|---|---|---|
| `node-pty` (C++ addon, ABI-compiled per platform) | Requires toolchain; breaks inside compiled binaries | `Bun.Terminal` — native PTY built into the runtime |
| `better-sqlite3` (native addon) | Same problem | `bun:sqlite` — same API shape, builtin |
| `tsx` runner | Requires Node installed on the target machine | Compiled into the binary |

After migration the server has zero native addons, so `bun build --compile` produces a genuinely self-contained executable for macOS (arm64/x64) and Linux (x64/arm64/musl).

## Constraints

- **No native compilation before this plan is done.** The compile step is step 8 and exists only to verify; it is not the goal of this document.
- Behavior must not change for clients. Mobile, desktop, and CLI all talk HTTP/WebSocket to this server; wire behavior must stay identical.
- Keep changes minimal per AGENTS.md. Each step below is independently testable and committable.

## Current state

- Entry point: `apps/server/index.ts`, run via `tsx watch index.ts` (dev) / `tsx index.ts` (start).
- Native deps: `node-pty` used only by `apps/server/api/src/terminal/pty.manager.ts`; `better-sqlite3` used by 7 files under `apps/server/agent/src/session/`.
- Daemon mode already exists (`CONSOLE_DAEMON=true`, logs under `~/.console/logs`, managed by `apps/cli/daemon-manager.ts`).
- Runtime deps are otherwise pure JS: hono, ws, zod, ai, @ai-sdk/openai-compatible, zod-to-json-schema.

## Steps

### 1. Run the server under plain Bun (no code changes)

- Install Bun and run `bun apps/server/index.ts` (and `bun --watch` for dev).
- Fix whatever surfaces: most likely candidates are `tsx`-specific module resolution and ESM/CJS interop edge cases.
- Verify: server boots, sessions list, run a chat turn, open a terminal session.
- Update `package.json` scripts and Makefile targets (`dev-server`, `dev-console`) to use `bun` instead of `tsx`.

### 2. Swap `better-sqlite3` → `bun:sqlite`

Files: all importers under `apps/server/agent/src/session/` (storage, schema, session-ops, projects, model-favorites, session-helpers, utils).

- `bun:sqlite` mirrors the better-sqlite3 API (`new Database(path)`, prepared statements, transactions). Most call sites should be import-path-only changes.
- Watch for: boolean binding semantics, `pragma` return shapes, and any better-sqlite3-specific options objects.
- Verify: existing session tests pass (`cd apps/server && npx tsx tests/<name>.test.ts` — switch test runner to `bun test` or keep tsx during transition), plus a manual dev-server run against an existing `~/.console-dev` database to confirm schema compatibility.
- Remove `better-sqlite3` and `@types/better-sqlite3` from `apps/server/package.json`.

### 3. Swap `node-pty` → `Bun.Terminal`

File: `apps/server/api/src/terminal/pty.manager.ts` (the only consumer).

- Keep the manager's public interface unchanged; replace only the PTY implementation behind it.
- Mapping:
  - spawn shell → `new Bun.Terminal({ cols, rows, data })` + `Bun.spawn([shell], { terminal })`
  - write input → `terminal.write(data)`
  - resize → `terminal.resize(cols, rows)`
  - exit handling → `await proc.exited` / close callbacks
- Explicitly verify behaviors the current code depends on:
  - env/cwd passthrough when spawning
  - output chunking/backpressure feel over the WebSocket (vim/htop from mobile)
  - SIGINT/Ctrl+C, raw mode, and clean teardown when the socket drops
- Note: pty.manager.ts currently chmods node-pty's prebuilt binaries at startup — that whole workaround is deleted.
- Remove `node-pty` from `apps/server/package.json`.

### 4. Audit remaining Node-specific usage

- Grep for APIs known to differ under Bun: `createRequire`, dynamic `require`, worker_threads, native `fs` watchers (`fswatch.service.ts`), `node-pty` leftovers.
- Confirm all deps work under Bun: hono, ws, zod, ai, @ai-sdk/openai-compatible, zod-to-json-schema, @ff-labs/fff-node.
- Decide the story for tests: either run the suite with `bun test` or keep tsx for tests only. Tests do not need to ship in the binary.

### 5. Lock Bun as the required runtime

- Pin a minimum Bun version in docs/CI (`bun >= 1.4`, wherever Terminal stabilized).
- Add a startup version check or document it in the README/Makefile.
- Update AGENTS.md test commands if the test runner changes.

### 6. Full regression pass

- Dev server against real storage dir; create session, chat, tools (bash/edit/read), todo, subagent.
- Terminal over WebSocket from mobile app and desktop app, including resize.
- Git routes, fs routes, provider auth flows (at least one OAuth provider).
- Daemon mode start/stop via the CLI, log file written.

### 7. Clean up

- Remove tsx from devDependencies, remove Node-version assumptions from scripts.
- Update Makefile help text.
- Commit history should read as one commit per step above.

### 8. Verification compile (gate check, not the goal)

- Run `bun build --compile` for the host platform only and smoke-test: binary starts as daemon, prints port, one chat turn, one terminal session.
- This step exists solely to prove the codebase is compilation-ready. If anything here fails, fix the cause in steps 1–7 — do not paper over it with compile-time hacks. Full cross-platform builds, installer script, daemon lifecycle hardening, and token auth belong to the separate single-binary distribution plan.

## Risks

- `Bun.Terminal` is new (Bun ~1.4). Mitigation: the swap is isolated to `pty.manager.ts`; node-pty can be restored behind the same interface if a blocker appears.
- SQLite file compatibility between better-sqlite3 and bun:sqlite is expected (same engine) but must be confirmed against a real `~/.console` database before removing the old dep.
- Windows support for `Bun.Terminal` is unverified. Out of scope unless/until the daemon targets Windows servers.
- `ai` SDK streaming under Bun's fetch/event-loop differs subtly from Node in some versions — the full regression pass in step 6 covers this.
