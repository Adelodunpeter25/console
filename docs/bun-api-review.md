# Bun API Review — replacing Node APIs with Bun-native equivalents

Review of `apps/server` + `apps/cli` after the runtime migration. The codebase now *runs* on Bun, but most I/O still goes
through Node-compatibility shims. Every shimmed API works — the point of this
review is where Bun's native APIs are meaningfully faster, lower-level, or let us
delete dependencies outright.

Already done (no action needed): `bun:sqlite` (all 7 session modules), `Bun.Terminal`
(`pty.manager.ts`), single-binary compile (`make build-server`).

---

## Priority 1 — hot paths, drop-in replacements

### 1.1 SHA-256 hashing on every persisted message

- **Where:** `apps/server/agent/src/session/session-messages.ts:44` and `:75` (`crypto.createHash("sha256")`)
- **Why it matters:** This runs for *every* message written to SQLite (content hashing for IDs/dedup). It is the single hottest crypto call in the server.
- **Fix:** `new Bun.CryptoHasher("sha256").update(JSON.stringify(safeMsg)).digest("hex")` — Bun's hasher avoids the Node compat layer and streams updates without copying.

```ts
// before
const hash = crypto.createHash("sha256").update(JSON.stringify(safeMsg)).digest("hex").slice(0, 32);
// after
const hash = new Bun.CryptoHasher("sha256").update(JSON.stringify(safeMsg)).digest("hex").slice(0, 32);
```

Same one-liner applies to `apps/server/providers/src/codex/oauth.ts:76` (PKCE challenge).

### 1.2 Shell execution inside the bash tool

- **Where:** `apps/server/agent/src/tools/bash.ts:69` (`exec(command, { maxBuffer: 10MB })`) — this is the core agent tool; *every* command the model runs passes through here.
- **Problems today:** `node:child_process.exec` buffers stdout/stderr in JS, emulates kills with signals across the shim, and `maxBuffer` overflow silently truncates output.
- **Fix:** `Bun.spawn` with inherited pipes and an explicit byte cap we control:

```ts
const proc = Bun.spawn(["bash", "-lc", command], { cwd, env, signal, stdout: "pipe", stderr: "pipe" });
const out = await new Response(proc.stdout).bytes(); // cap by slicing if > limit
```

Bonus: `proc.kill()` maps directly to the existing timeout/AbortSignal logic, and exit codes come back as plain numbers instead of nullable shim fields (`exitCode === null && !killed` juggling at lines 110–111 disappears).

### 1.3 Git service exec wrapper

- **Where:** `apps/server/api/src/services/git.service.ts:1-11` (`promisify(exec)` used for every git status/branches/log call)
- **Fix:** same `Bun.spawn` helper as 1.2 (extract once into e.g. `api/src/utils/exec.ts`, use from both). Git status is polled by UIs — latency users feel.

---

## Priority 2 — user-facing file I/O

### 2.1 File reads/writes in the FS browser service

- **Where:** `apps/server/api/src/services/fs.service.ts:181` (`fs.readFile`), `:196` (`fs.writeFile`) — these serve the mobile/desktop Files screens.
- **Fix:** 

```ts
const text = await Bun.file(filePath).text();
await Bun.write(filePath, content); // atomic by default
```

`Bun.write` also gives atomic-rename semantics for free (no torn writes if the client disconnects mid-save).

### 2.2 System-prompt / skill discovery reads

- **Where:** `apps/server/agent/src/systemprompt/walk.ts:68` (`readFile`), `:97` (`readdir withFileTypes`), plus `listMarkdownFiles`. Runs at session start for AGENTS.md/skills/rules discovery.
- **Fix:** `await Bun.file(p).text()`; directory listing can stay on `fs.readdir` (Bun's readdir is already optimized) — only swap the file reads.

### 2.3 Synchronous stat/exists checks on the request path

- **Where (15 call sites):** `server/api/src/terminal/pty.manager.ts` (`existsSync` before spawn), `server/agent/src/session/projects.ts`, `session-ops.ts`, `session-helpers.ts`, `server/api/src/services/assist.service.ts`
- **Why:** each `existsSync` blocks the event loop; several sit inside request handlers.
- **Fix:** prefer `existsSync` only at startup; on request paths use `await Bun.file(p).exists()` (async, cached by Bun's FS cache). Lowest urgency of the three above.

---

## Priority 3 — architectural wins (bigger, do deliberately)

### 3.1 Replace `node:http` server + manual Hono bridging with `Bun.serve`

- **Where:** `apps/server/index.ts:5,69,129` — `createServer(async (req,res) => ...)` hand-wiring Hono's fetch handler onto the Node server object.
- **Fix:**

```ts
Bun.serve({
  port, hostname,
  fetch: app.fetch,
});
```

Removes ~60 lines of manual req/res bridging and daemon shutdown wiring simplifies (server.stop()). All Hono middleware stays identical.

### 3.2 Replace `ws` with Bun's native WebSockets

- **Where:** `apps/server/api/src/terminal/socket.route.ts:17,41` (`WebSocketServer({ noServer: true })` attached to the Node http.Server) — carries all terminal traffic between mobile/desktop and the PTYs.
- **Why:** terminal sessions stream continuously while typing; the `ws` shim does per-message JS allocations that Bun's C++-level publish path skips. Also deletes the `ws` dependency entirely.
- **Fix:** fold into 3.1 — `Bun.serve({ websocket: { message(ws, data) {...}, close() {...} } })`, route upgrades via the same server. The backpressure pause/resume protocol in `socket.route.ts` maps to checking `ws.getBufferedAmount()` (native).
- **Risk:** moderate — the terminal protocol (spawn frames, resize, kill, backpressure) needs careful porting; keep `tests/terminal.test.ts` as the gate.

### 3.3 OAuth login callback servers

- **Where:** `apps/server/providers/src/auth/login.ts:11` (`node:http` server listening locally for provider OAuth callbacks)
- **Fix:** `Bun.serve({ fetch })` one-liner; low priority (runs rarely, briefly).

---

## Keep as-is (checked, no benefit)

| API | Where | Verdict |
|---|---|---|
| `node:path`, `node:os` (32+7 imports) | everywhere | No faster alternative; Bun's is the same code |
| `node:util` promisify | git.service | Disappears naturally with 1.3; otherwise harmless |
| `node:events` EventEmitter | notification.service, fswatch.service | Bun's is compliant and fast; not worth churn |
| `fs.watch(recursive)` | fswatch.service.ts | Native on Bun; revisit only if events misbehave |
| `node:string_decoder` | tools/read/engine.ts | Could use `TextDecoder({stream:true})`; zero measurable difference at our chunk sizes |
| zod, commander, hono middleware | everywhere | Runtime-agnostic pure JS |

## Random UUIDs (optional micro-win)

9 files import `randomUUID` from `node:crypto` (`agent-loop.ts`, `tool-executor.ts`, `ask.ts`, `pty.manager.ts`, providers...). `crypto.randomUUID()` global works on Bun and is marginally faster than the module import path. Volume is too low to matter except per-tool-call IDs; fine to sweep opportunistically, never urgent.

---

## Suggested order

1. **1.1 CryptoHasher** — one-line, hottest path (message hashing)
2. **1.2 + 1.3 shared spawn helper** — bash tool + git service (most user-visible latency)
3. **2.1–2.3 Bun.file swaps** — mechanical, testable via existing suites
4. **3.1 Bun.serve** — contained to index.ts
5. **3.2 Native WebSockets** — biggest win, biggest risk; gate on terminal tests
6. **3.3 + UUID sweep** — cleanup pass

Each step keeps `tests/*.test.ts` green under `bun tests/<file>.test.ts` per repo rules.
