# Bun API Review — replacing Node APIs with Bun-native equivalents

Review of `apps/server` + `apps/cli` after the runtime migration. The codebase now *runs* on Bun, but most I/O still goes
through Node-compatibility shims. Every shimmed API works — the point of this review is where Bun's native APIs are meaningfully faster, lower-level, or let us delete dependencies outright.

Already done (no action needed): `bun:sqlite` (all 7 session modules), `Bun.Terminal`
(`pty.manager.ts`), single-binary compile (`make build-server`).

---

## ✅ DONE — Priority 1 — hot paths, drop-in replacements

### 1.1 SHA-256 hashing on every persisted message — DONE (`8812fc9`)

- **Where:** `apps/server/agent/src/session/session-messages.ts:44` and `:75`, plus `providers/src/codex/oauth.ts:76` (PKCE challenge)
- **What changed:** all three `crypto.createHash("sha256")` calls now use `new Bun.CryptoHasher("sha256")`.

### 1.2 Shell execution inside the bash tool — DONE (`94aea23`)

- **Where:** `apps/server/agent/src/tools/bash.ts`
- **What changed:** `node:child_process.exec` replaced with `Bun.spawn` via a shared helper (see 1.3). `maxBuffer` overflow no longer kills data mid-stream — capture switches to drain-and-discard past the cap so children terminate normally and report real exit codes; SIGTERM→SIGKILL escalation and AbortSignal handling preserved.

### 1.3 Git service exec wrapper — DONE (`94aea23`)

- **Where:** shared helper extracted once into `api/src/utils/exec.ts`; used by both the bash tool and git.service.
- Exports:
  - `spawnCapture(argv, { cwd, env, timeoutMs, signal, maxBytes })` → `{ stdout, stderr, exitCode, killed, aborted }`
  - `execShell(command, options)` → mirrors the old `promisify(exec)` contract (throws on non-zero exit with `.code`/`.stdout`/`.stderr` attached) so existing error-matching call sites keep working.

---

## ✅ DONE / ⏭ SKIPPED — Priority 2 — user-facing file I/O

### 2.1 File reads/writes in the FS browser service — DONE (`4f3a83c`)

- `fs.service.ts` reads use `await Bun.file(filePath).text()`; writes use `await Bun.write(filePath, content)` (atomic by default).

### 2.2 System-prompt / skill discovery reads — DONE (`4f3a83c`)

- `walk.ts` `readTextFile` uses `Bun.file(p).text()`; directory listing stayed on `fs.readdir` per the original recommendation.

### 2.3 Synchronous stat/exists checks on the request path — SKIPPED (deliberate)

- The 15 `existsSync` sites guard SQLite DB files in modules that already run synchronous `bun:sqlite` queries on the same paths — event-loop blocking is an accepted trade-off there, and async conversion would ripple function signatures for microsecond-level gains. Revisit only if profiling ever flags these.

---

## ✅ DONE — Priority 3 — architectural wins

### 3.1 `Bun.serve` replaces node:http bridging — DONE (`cf17f3e`)

- `index.ts` lost the manual IncomingMessage↔Request bridging entirely; Hono's `app.fetch` is served directly.

### 3.2 Native WebSockets for terminals — DONE (`cf17f3e`)

- `socket.route.ts` rewritten for Bun.serve's native websockets (`server.upgrade` for `/api/terminals`).
- Backpressure mapped from `ws.bufferedAmount`/`drain` to `getBufferedAmount()` + a low-water poller (25ms interval while paused).
- Per-socket state (session id, pause flag, poller) lives in upgrade-time `data`.
- The `ws` dependency was **deleted**; `tests/terminal.test.ts` was ported to boot a real Bun.serve instance and is the gate for this path (spawn/echo/resize/kill verified).

### 3.3 OAuth login callback servers — DONE (`fbb058c`)

- `providers/src/auth/login.ts` `startCallbackServer` now uses `Bun.serve({ fetch })`.

---

## Keep as-is (checked, no benefit)

| API | Where | Verdict |
|---|---|---|
| `node:path`, `node:os` (32+7 imports) | everywhere | No faster alternative; Bun's is the same code |
| `node:util` promisify | git.service | Removed naturally with 1.3 |
| `node:events` EventEmitter | notification.service, fswatch.service | Bun's is compliant and fast; not worth churn |
| `fs.watch(recursive)` | fswatch.service.ts | Native on Bun; revisit only if events misbehave |
| `node:string_decoder` | tools/read/engine.ts | Could use `TextDecoder({stream:true})`; zero measurable difference at our chunk sizes |
| zod, commander, hono middleware | everywhere | Runtime-agnostic pure JS |

## Random UUIDs (optional micro-win)

9 files import `randomUUID` from `node:crypto`. Volume is too low to matter except per-tool-call IDs; fine to sweep opportunistically, never urgent.

---

## Status summary

1. ~~**1.1 CryptoHasher**~~ ✅ `8812fc9`
2. ~~**1.2 + 1.3 shared spawn helper**~~ ✅ `94aea23`
3. ~~**2.1–2.3 Bun.file swaps**~~ ✅ 2.1–2.2 done (`4f3a83c`); 2.3 skipped deliberately
4. ~~**3.1 Bun.serve**~~ ✅ folded into 3.2 (`cf17f3e`)
5. ~~**3.2 Native WebSockets**~~ ✅ `cf17f3e` — gated on `bun tests/terminal.test.ts`
6. **3.3 OAuth callbacks** ✅ `fbb058c`; UUID sweep left as opportunistic cleanup
