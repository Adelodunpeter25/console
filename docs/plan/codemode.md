# Codemode — let the model orchestrate our own tools in JavaScript

Add a `code` tool whose sandbox runs **on the harness side** and lets the model
call *our* tools as JavaScript functions. One model turn becomes hundreds of
tool executions, and the model's context gets a compact log instead of three
hundred tool results. This is Pi's Codemode, applied to the harness's own tool
set instead of to one driver.

`https://earendil.com/posts/pi-1-0/` and
`https://earendil.com/posts/you-said-no-mcp/` are the reference for the shape
of the problem — a sandbox on the trusted side, the tool set exposed as
functions, orchestration instead of round trips — not a stack to copy. We are
not bringing in QuickJS, WASM, or a second language runtime.

**We already built this once.** `internal/services/cua/js.go` is a Codemode for
the computer-use driver: one persistent goja interpreter per chat session, the
driver API exposed as methods on a `cua` object, and two harness tools
(`computer`, `computer_reset`) instead of 56 tool schemas re-sent every turn.
The comment in that file states the motivation in Pi's terms — *"the per-turn
schema cost stays flat no matter how many methods the driver grows; API
knowledge is read once, not re-sent as definitions."* This plan generalizes
that pattern from one driver to the harness's own tools. The runtime, the tool
shape, the skill-as-documentation layer, the per-session state and the deferred
loading (`loadTools`) all exist.

Target: `apps/server-go/internal/agent/codemode/` (runtime, tool exposure,
skill text), `apps/server-go/internal/run/` for activation and per-session
ownership, `apps/server-go/internal/agent/loop/executor.go` for the call path,
and `apps/server-go/tests/codemode/` for coverage.

---

## 0. Non-Goals

- **No new language runtime.** goja is already a dependency (via
  `internal/services/cua`) and is pure Go, so the server keeps building
  everywhere it builds today, including Linux, with no new native dependency.
  Pi ships a WASM-embedded JS for protection; we accept the CUA precedent for
  the same threat model (see §2.5).
- **No AST codemod engine.** This is not jscodeshift, `cargo fix`, or
  `gofmt -r`. We are not building a structural rewriter; the model writes
  ordinary JavaScript that calls our tools. The roadmap's old example ("the
  import-alias migration") is stale — there is no TypeScript left in the tree —
  and it was a *script* example, not an engine example.
- **No privilege escalation.** The sandbox may only call tools the model could
  already call, at or below their tier. A script cannot reach a tool that is
  not in its exposed surface.
- **No bypassing of approvals, the path guard, or the file-change snapshot.**
  Those live in the call path, not in the tool's registration, so they apply to
  calls made from inside a script exactly as they apply to direct calls.
- **No replacement for the direct tools.** `read_file` and friends stay
  available directly: a single file read is a tool call, not a script. The
  point is the case where one call would become hundreds.
- **No always-on tool.** Like `/computer-use`, the feature is activated
  explicitly. A permanent `code` tool in every turn is a permanent schema cost
  and a permanent prompt-injection surface.
- **No `bash` inside the sandbox in v1.** `bash` is the untrusted side; a
  trusted-side script that can spawn a shell collapses the two sides. Revisit
  only if a real workflow needs it, and then as an explicit stage-3 decision.
- **Not a subagent feature.** Subagent transcripts fold into a summary; a
  script that ran there would be invisible to the user.

---

## 1. Sequence: read-only first, writes later

**Recommendation: stage 1 is read-only, and it is worth shipping on its own.**

1. **Stage 1 — read-only codemode.** A `code` tool whose sandbox exposes the
   read tools (`read_file`, `list_dir`, `glob`, `grep`) plus `print`. Zero new
   write risk. This is where the value shows first: Pi's own example — boiling
   a week of commits into a short summary — is entirely read-only.
2. **Stage 2 — writes in shadow mode.** The write tools join the surface, but
   every write is *recorded, not applied*; the script gets back the would-be
   diff. This is the "scripted bulk refactors, safely" half.
3. **Stage 3 — apply and verify.** Writes apply under one approval per script;
   the script can call the build and test tools and check the result itself,
   and the harness rolls back when verification fails.

Why read-only first: the write path is where the risk and the complexity live
(approvals mid-script, shadowing, rollback), and it is also where the value is
most contested. Stage 1 proves the orchestration model, the context saving and
the tool-surface design on the half that cannot break anything.

Why not stage 1 with writes already: a script that can write is a script that
can write 300 files before the model sees any of it. The dry-run mode in stage
2 exists precisely because that is the failure mode worth designing against, and
it should be designed, not retrofitted.

---

## 2. Why this shape

### 2.1 The sandbox has no tools of its own

The JavaScript runtime gets no filesystem, no network, no `require`, and no
process access. Its entire capability surface is the set of functions we
install. That is what makes the security argument simple: **the script cannot
do anything the harness has not explicitly handed it**, and everything it can
do is a call that goes through the existing executor.

### 2.2 Every call inside the script is an ordinary tool call

`Executor.Execute` (`agent/loop/executor.go`) already does, in order:

```
registry.Get(name) → permissions.Resolve(mode, tier) → approver.Approve(...)
                   → Guard.Check(call) → OnBeforeExecute(call, tool) → execute
```

A call made from inside the sandbox goes through the same path. Three things
follow for free:

- **Tiers still gate it.** `Resolve(mode, tier)` returns allow / prompt /
  deny from the approval mode, exactly as for a direct call.
- **Writes stay inside the project root.** `Guard.Check` confines write-capable
  calls to the session's project root.
- **The desktop's diff view keeps working.** `OnBeforeExecute` snapshots the
  pre-write content for the session's file-change diff. A file edited by a
  script shows up in the client's change review exactly like a file edited by
  `edit_file`.

The alternative — a sandbox with its own file-writing path — would have to
re-implement all three. That is the reason the sandbox calls *our* tools
instead of doing the work itself.

### 2.3 One runtime per session, state in the transcript

The CUA runtime is one persistent goja interpreter per chat session
(`cua.JSRuntime`, owned by `cua.Manager.sessionRuntime`), so variables and
bindings survive between evaluations until `computer_reset`. The codemode
runtime does the same, so a script can build up state across calls — a file
list, a set of planned edits — without re-deriving it.

Pi keeps the same state in the session transcript rather than the filesystem.
We should match that where it is free: the state a script leaves behind belongs
to the session record, not to a temp directory that outlives it.

### 2.4 The catalogue is documented once, not re-sent as schemas

This is the CUA lesson and it is the one with numbers. The driver has 56
methods; registering them as harness tools cost ~28K tokens re-sent on every
turn. Two tools and a skill text cost the same forever.

So the codemode surface is:

- **`code`** — run JavaScript against the session's runtime.
- **`code_reset`** — clear the session kernel (read-tier, like `loadTools`).

and the catalogue of exposed functions is documented **once**, in a skill, the
way `cua.SKILL.md` documents the driver API. Tool metadata decides whether a
tool is visible to the model directly, deferred, or codemode-only — Pi's point
in "You Said No MCP!", and we already have the deferred half (`loadTools` for
MCP groups).

### 2.5 goja, not WASM

Pi ships a WASM-embedded JS so the sandbox cannot reach the host. We already
accepted goja for model-authored JavaScript in this product: the CUA runtime
runs in the server process, and it is confined by giving the script no
capabilities beyond the driver API. The same confinement applies here, and the
threat model is the same: model-authored code that can only call functions we
installed. Adding a WASM runtime to solve a problem the existing precedent
already solved in this codebase would be a second runtime for no new safety.

What we do owe the model: a wall-clock budget (CUA uses 5 minutes), an output
cap, a memory ceiling, and a promise detector that fails loudly rather than
silently doing nothing — goja runs no microtask pump between evaluations, so
`Promise.all` would be a no-op that *looks* like it worked. CUA already
detects and rejects that case; the codemode must do the same, loudly.

### 2.6 The sandbox's tier is the highest tier it exposes

A read-only codemode is a read tool. A codemode that can write is a write
tool. A codemode that can shell out is an exec tool. The tier is a property of
the *exposed surface*, not a separate policy, so `permissions.Resolve` keeps
working unchanged and the approval prompt names the real risk.

---

## 3. The tool surface

### 3.1 The `code` tool

```jsonc
{
  "code": "const files = await tools.glob({ pattern: '**/*.rs' });\n" +
          "let n = 0;\n" +
          "for (const f of files) { if ((await tools.read_file({ path: f })).text.includes('TODO')) n++; }\n" +
          "print(`${n} files with TODOs`);\n" +
          "return { n };",
  "timeout_ms": 120000
}
```

- `code`: the JavaScript to run. Bounded in size (CUA caps at 200KB).
- `timeout_ms`: the evaluation budget. Defaults to 5 minutes, like CUA.

The tool returns the script's completion value plus a compact log of the calls
it made (`✓ read_file {"path": "..."} 12ms`), the way Pi reports them. The
model sees the log, not the raw results of every call — that is where the
context saving comes from.

### 3.2 The JavaScript surface

| Member | Meaning |
|---|---|
| `tools.<name>({...})` | Call an exposed harness tool. One function per tool, taking the tool's own argument object. Returns `{ content, structuredContent, isError }` — the same shape CUA scripts see. |
| `print(...)` | Narration. Collected and reported alongside the completion value. |
| `store(key, value)` / `load(key)` | Session-scoped state that survives between evaluations (Pi's `store`). Backed by the session transcript. |

There is no `require`, no `fetch`, no filesystem, and no timer. A script that
tries gets a `ReferenceError`, which is the point: the capability surface is
the tools.

### 3.3 Exposed tools, by stage

| Stage | Exposed | Tier of `code` |
|---|---|---|
| 1 | `read_file`, `list_dir`, `glob`, `grep` | read |
| 2 | + `write_file`, `edit_file`, `batch_write` (shadow mode) | write |
| 3 | + apply, verify, rollback; `bash` only if a workflow demands it | write / exec |

Each exposed tool keeps its own schema and validation: the JS function takes
the same object the tool's JSON schema describes, and the same decoder and
validator run. A malformed call fails the same way a direct call fails.

---

## 4. Execution model

```
code tool call
  └─ codemode.Runtime.Eval(ctx, code, budget)
       └─ goja runtime (one per session, mutex-guarded)
            └─ tools.read_file({...})  →  Executor.Execute(call)
                                          ├─ permissions.Resolve(mode, tier)
                                          ├─ approver.Approve(...)      ← may block
                                          ├─ Guard.Check(call)           ← project root
                                          ├─ OnBeforeExecute(...)        ← file-change diff
                                          └─ the tool itself
```

- **The script's calls are ordinary calls.** `Executor.Execute` is the same
  entry point the direct tool path uses, so the approval prompt, the path
  guard and the change snapshot all apply without special cases.
- **Approvals can block mid-script.** The approver is asynchronous
  (`permissions.Approver`), so a script can await a call that needs a
  decision. The open question (§6) is the *granularity*: one approval per
  script, or per call. Per call is faithful and unusable at 300 writes; one
  per script is the sane default, but it must be a decision the user actually
  made, with the script's surface visible in the prompt.
- **The runtime is mutex-guarded**, like CUA's. goja is not safe for concurrent
  use, and a script's own `Promise.all` would issue concurrent tool calls
  against one runtime. Decide explicitly whether concurrent calls are
  serialized at the tool boundary or refused; do not let it happen by
  accident.
- **Stage 2's shadow mode** intercepts write-tier calls: the tool runs, the
  write is captured instead of applied, and the script receives the
  would-be diff. The diff is what the model shows the user, and it is what the
  stage-3 apply step consumes.
- **Stage 3's verification** is the script's own: it can call the build and
  test tools and check the result, and the harness rolls back when the check
  fails. The harness does not need to know what "correct" means for a given
  codebase — the script does.

---

## 5. Verification

Go tests for the runtime live next to the package, the way
`internal/run/background_test.go` and `internal/routes/run_test.go` do; the
end-to-end behaviour (a script editing files, the approval flow, the change
diff) belongs in `apps/server-go/tests/`.

- **The runtime, with a fake caller.** A `JSCaller`-shaped fake (the CUA test
  seam) records the calls a script makes, in order, with their arguments. This
  proves the orchestration model: a loop issues N calls, a `Promise.all` issues
  them concurrently or is refused, and a thrown error surfaces as a JS
  exception rather than a harness abort.
- **The call path is not bypassed.** A script that calls a write tool in
  `always-ask` mode produces exactly one approval request; in `full-access`
  mode it produces none; a call outside the project root is refused by the
  guard even from inside a script.
- **The change snapshot still fires.** A file written by a script appears in
  the session's file-change diff (`OnBeforeExecute`), so the desktop's change
  review shows it.
- **The budget holds.** A runaway script is stopped at the budget and reported
  as an unknown completion, the way CUA reports it.
- **The promise detector fails loudly.** `Promise.all` in stage 1 either runs
  or is refused with a clear message; it must never silently do nothing.

---

## 6. Open questions

1. **Approval granularity.** One approval per script (with the exposed surface
   shown) is the sane default; per call is faithful and unusable. Decide
   before stage 2, not during it.
2. **Concurrency.** Serialized tool calls, or genuinely concurrent ones? The
   runtime is not concurrency-safe, so this is a decision, not an accident.
3. **Where the state lives.** Pi keeps script state in the session transcript.
   Matching that is free if the runtime is per session; it is work if the
   state is meant to survive a restart.
4. **Whether the direct tools stay.** The point of Codemode is a smaller
   per-turn schema cost, which argues for moving *all* the read tools behind
   it eventually — but that would make the common single-read case worse.
   Revisit with real numbers from stage 1.
5. **Activation.** `/codemode` (mirroring `/computer-use`) is the
   conservative choice. Whether the harness should enable it automatically
   for long-horizon runs is a later question.
