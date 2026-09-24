# Harness Token Efficiency

Status: **planned**.

## Goal

Lower the token cost of each **completed task** in the Go agent harness
(`apps/server-go`) without making the agent worse at its job.

- **Objective:** fewer price-weighted tokens per completed task.
- **Constraint:** no drop in task success on our fixed benchmark tasks.

Measure per task, not per request. Every turn resends the whole prefix (tool
definitions, system prompt, conversation so far). A change that makes each
request smaller but adds turns can cost more overall.

Background: Cursor's write-up "Improved token efficiency for longer agent
runs" (Sep 2026). They cut overall token cost about 7% by trimming the
system prompt (about 66% shorter), loading rarely used tools on demand (60%
fewer tool-description tokens), fixing their cache layout (20% fewer cold
cache misses), numbering only every 10th line in file reads (1.6% fewer
cache-read tokens), and tuning subagents. Use these numbers to judge rough
size, not as targets.

## Principles

1. **Change what the harness sends, not how hard the model tries.** Never
   tell the model to "save tokens" or "be brief". That makes models give up
   on ambitious work.
2. **Describe; don't command.** Replace MUST, NEVER and `<critical>` with
   plain descriptions of what a tool does. Keep instructions only for things
   the model can't know (product, environment, the user's process) and for
   quirks we've actually seen in transcripts.
3. **Static context is for what most turns need.** Everything else should be
   something the model can look up.
4. **Expect removals to win.**
5. **Never truncate silently.** Large output goes to a file, and the model
   gets a path and a tail.
6. **One change per commit, and every change can be rolled back.** Risky
   changes go behind a flag.

## Current state (audit, 2026-09-24)

| Area | Where | Finding |
|---|---|---|
| Prompt assembly | `internal/agent/systemprompt/builder.go`, called from `internal/run/turns.go:215` | One flat string: identity → approval mode → skills → rules → **tool inventory** → slash commands → AGENTS.md → **workspace tree** → **workstation (date, cwd, branch, model)** → `<critical>`. Things that change (date, branch, tree, model) sit inside the cached prefix. |
| Tool inventory | `renderTools` | Repeats the tool names that are already in the tool schemas. Pure duplication. |
| Emphasis | `renderContextFiles`, `renderWorkstation` | "You MUST follow…", "Do not waste tools…", `<critical>` "MUST advance the task". |
| Workspace tree | `workspacetree.go` | Depth-limited tree on every request, even though `glob`/`list_dir` exist. |
| Claude cache | `providers/claude/convert.go:251,316`, `stream.go:150` | Breakpoints on the system block, the last tool, and the last message. The system block is one big string, so any change in date/branch/tree throws away the cache for everything after the tools. |
| OpenCode cache | `providers/opencode/stream.go` | Responses path sends `prompt_cache_key`. **The Chat path (`runChat`, used by `space-bunny-free`) sends no cache key.** |
| **OpenCode system prompt** | `providers/opencode/stream.go:100–112` | **Bug: `runChat` never sends `req.SystemPrompt`.** Only `runResponses` sets `instructions`. Chat-model runs today have no system prompt at all. |
| Usage telemetry | `loop.TurnUsage` (`agent/loop/message.go:66`), each provider's `NormalizeUsage` | Per-turn input, cache read/write and output are recorded on the assistant message, but never added up per task, per tool, or per context source. `run/turns.go:380` swallows `EventUsage`. |
| File reads | `tools/file_tools.go:120–123` | Every line is numbered (`%*d: `). |
| Bash output | `tools/bash_tools.go:17,30–34` | Keeps the first 50 KB and drops the rest. Errors usually show up at the **end**, which is the part that gets thrown away. |
| Firecrawl / webFetch | `tools/firecrawl.go:22–29` | Also cuts off the head without saving the rest. |
| Tools shipped | `tools.DefaultTools()` + memory | 17 tools with full schemas on every request (`webSearch`, `webFetch`, `bashJob`, `askMany`, `memory`, `batchWrite` are rarely used). OpenCode adds 6 compatibility marker tools on top. |
| Compaction | `agent/compaction/llm.go:24` | Summary prompt is already short. The full history isn't saved anywhere the agent can search. |
| Subagent | `agent/loop/subagent.go:129` | Adds a fixed preamble to the parent's **whole** system prompt (tree included). Returns the child's entire streamed text as its "summary". |

## Test setup: OpenCode free model

All cost measurements use the OpenCode Zen provider with a free model
(`space-bunny-free`, found via `GET /zen/v1/models`), so we can run it as
often as we want at no cost.

Caveats:

- Free models cost $0, so we compare **price-weighted tokens**. We use a
  fixed reference price table (per million tokens), kept in the bench tool:
  - uncached input: 1.00
  - cached input: 0.10
  - cache write: 1.25
  - output: 5.00

  Only the ratios matter; they roughly match current frontier pricing.
- Zen Chat Completions may not report cached tokens
  (`NormalizeChatUsage` returns `CacheUnsupported`). When it doesn't, the
  bench tool must also report a **local estimate** of the prefix that would
  be cacheable: bytes shared with the previous request of the same run,
  converted with `compaction.EstimatePayloadTokens`. That way cache-layout
  changes can still be measured on a free model.
- Free-model quality is lower and noisier than frontier models. Run each task
  **3 times** and compare medians. Before we ship Phase 4 flags as defaults,
  do one confirmation run on a paid Claude or Codex model, if one is
  available.
- Rate limits: run tasks one after another, with a pause between them.

---

## Phase 0: Fix the OpenCode baseline (prerequisite)

We can't measure the harness on OpenCode until the system prompt actually
gets sent.

### Task 0.1: Send the system prompt on the Chat path
- **File:** `internal/providers/opencode/stream.go` (`runChat`), and
  `messages.go` if the system message is built there.
- **Change:** when `req.SystemPrompt` isn't blank, put
  `{"role":"system","content":req.SystemPrompt}` first in `messages`.
- **Test:** `tests/providers/opencode_provider_test.go`: add a Chat-model
  case that checks `messages[0]` is the system message.
  `cd apps/server-go && go test ./tests/providers/ -run OpenCode`
- **Rollback:** revert the commit.

### Task 0.2: Send a cache key on the Chat path
- **Change:** do what `runResponses` does. Send `prompt_cache_key =
  req.ConversationID` when caching is on. (Zen/OpenAI-compatible servers
  ignore fields they don't know about. Check that the request isn't
  rejected; if it is, drop this task.)
- **Test:** same test file; check the key is present on the Chat body.

---

## Phase 1: Telemetry and baseline (change directly)

Everything after this depends on it.

### Task 1.1: Token totals per run
- **Files:** `internal/agent/loop/loop.go`, new `internal/agent/loop/usage.go`.
- **Change:** keep a `RunUsage` on the agent that adds up every
  `TurnUsage` in a run: turns, input, cacheRead, cacheWrite, output,
  reasoning, plus the reference-weighted cost. At the end of a run, emit it
  once as a new internal event, or attach it to `EventTurnDone`. Subagent
  runs add their totals to the parent under a `subagents` field, so we can
  measure the whole tree.
- **Test:** `tests/agent/usage_test.go`. Use a fake provider that emits known
  usage and check the totals, including a nested subagent.

### Task 1.2: Per-source size breakdown
- **Files:** `internal/agent/systemprompt/builder.go` (already returns
  `Sections`), new `internal/agent/loop/breakdown.go`.
- **Change:** for each request, estimate tokens (`compaction` estimator) for
  each source:
  - system-prompt sections, by `Section.Name`
  - tool definitions, per tool
  - history, split into user text, assistant text, tool results (grouped by
    tool name) and compaction summaries

  Log this at debug level. The bench tool reads it.
- **Test:** `tests/agent/breakdown_test.go`, using a known prompt and history.

### Task 1.3: Per-tool stats
- **Change:** for each run, count how many times each tool was called, how
  many of those calls returned `IsError`, and how many bytes each tool
  returned. Classify errors as `invalid_args | env | provider | timeout |
  aborted | unknown`, based on the error type from the executor
  (`agent/loop/executor.go`) and the tool. Unknown errors are harness bugs.
  Log them.
- **Test:** `tests/agent/toolstats_test.go`.

### Task 1.4: Save rendered requests
- **Change:** when `CONSOLE_DUMP_REQUESTS=<dir>` is set, write each provider
  request body as JSON (`<run>/<turn>.json`), for all providers. Off by
  default. We read these dumps by hand to spot duplicated text, changing
  values and blocks in the wrong order.
- **Where:** one helper in `internal/providers/shared`, called from each
  provider's `postJSON` or equivalent.

### Task 1.5: Bench tool and task set
- **Files:** new `apps/server-go/cmd/bench/main.go`, and task files in
  `apps/server-go/bench/tasks/*.toml`.
- **Task set:** 12 realistic tasks written the way the user really writes
  (short, slightly vague), against a copy of this repo at a pinned commit
  (a git worktree under a temp dir). Mix:
  - 3 questions about the code ("where is the cache breakpoint set for claude?")
  - 3 small edits with a checkable result ("add a `Truncated` field to X")
  - 2 bug fixes with a failing Go test the task has to make pass
  - 2 multi-step tasks (read, then edit several files, then run a test)
  - 1 long-output task (run a noisy command, find the error)
  - 1 task that tends to trigger a subagent

  Each task has a `check` command (test, grep, or a file assertion) that
  decides success automatically.
- **Runner:** for each task, repeated N times (default 3): reset the
  worktree, run the agent loop in-process with the OpenCode provider and a
  free model, run `check`, and record success, turns, RunUsage, weighted
  cost, tool stats, wall time and the estimated cacheable prefix.
- **Flags:** `--model`, `--runs`, `--tasks`, `--flags key=value,...` (turns
  on harness feature flags, see Phase 4), `--out results.json`.
- **Output:** a table per task and overall: success rate, median turns,
  median weighted cost per **successful** task, tool error rate, cost share
  by source. Also `bench compare a.json b.json` to show the difference.
- **Test:** `tests/bench/bench_test.go` runs the runner with a fake provider
  (no network).

### Task 1.6: Record the baseline
- Run `go run ./cmd/bench --runs 3 --out bench/results/baseline.json` after
  Phase 0.
- Add a short "Baseline" section to this doc: cost share by source × billing
  type, static tokens per request, turns per task, and per-tool usage and
  error rate. Rank the rest of the work with it.

---

## Phase 2: Cache layout (change directly)

Keep the reusable prefix byte-for-byte identical across turns, and as long
as possible:

`tools → stable system prompt → [breakpoint] → setup message (env, tree, skills, rules, AGENTS.md) → [breakpoint] → conversation`

### Task 2.1: Split the system prompt into stable and volatile parts
- **File:** `builder.go`.
- **Change:** `Result` gets `StableSystem` and `Setup`.
  - **Stable:** identity, approval-mode instructions, what skills are for,
    and the `<critical>` block (rewritten in Phase 4).
  - **Setup** (moved out of the system prompt): skills list, rules, slash
    commands, AGENTS.md / context files, workspace tree, and workstation
    info (date, cwd, OS, arch, branch, model).
  - Keep `SystemPrompt` as the joined string, so callers that haven't been
    changed keep working.
- **Test:** `tests/agent/systemprompt_test.go`. Check that the stable part
  contains no date, cwd or branch, and is identical across two builds with
  different dates and branches.

### Task 2.2: Send setup as a leading user message
- **Files:** `agent/loop/loop.go` (`TurnRequest` gets a `Setup string`),
  `run/turns.go`, and each provider's message converter.
- **Change:** providers put `Setup` in a first user message
  (`<setup>…</setup>`) before the conversation. It's built once when the
  session starts and saved with the session, so it stays the same across
  turns and is only rebuilt when cwd, model or approval mode changes, or on
  compaction. Rebuilding the date every turn would break the cache; a date
  that's a few hours old is fine.
- **Where to cache it:** reuse the session header, or add an in-memory map
  keyed by session ID in `run`.
- **Test:** provider tests check the order: system, then setup, then
  history. A run test checks that setup stays the same across two turns.

### Task 2.3: Explicit breakpoints (Claude)
- **File:** `providers/claude/stream.go:146–150`.
- **Change:** the system prompt becomes two blocks: the Claude Code identity
  plus the stable part, with `cache_control` on the stable block. Put a
  second breakpoint on the setup user message, and keep the existing one on
  the last message. That's 4 breakpoints in total (tools, stable system,
  setup, tail), which is Anthropic's limit.
- **Test:** `tests/providers/claude_provider_test.go` checks the breakpoint
  positions.

### Task 2.4: Deterministic serialization
- **Change:** sort tool definitions by a fixed order (registry order, never
  map iteration). Check that `MergeToolDefinitions` and every converter keep
  that order. Check that JSON schemas serialize with stable key order (Go
  `encoding/json` sorts map keys; structs follow field order).
- **Test:** build the same request twice and assert the bytes are
  identical. Use the Task 1.4 dumps to confirm between real turns.

### Task 2.5: Don't switch models mid-conversation
- **Change:** the vision fallback in `run/turns.go:203` swaps the model for
  the whole turn, which throws away the cache. Run it as a one-off subagent
  call (describe the image, return text) instead of switching the main
  model. **Proposal only** until measured, because it changes behavior.

### Validate Phase 2
Run bench against the baseline. Expect: the same success rate, and a much
higher cacheable-prefix share from turn 2 onward (locally estimated on Zen).
Where the provider reports it, expect a higher cache-read share.

---

## Phase 3: Tool output (change directly)

### Task 3.1: Save large output to a file instead of cutting it off
- **Files:** `tools/bash_tools.go`, new `tools/spill.go`.
- **Change:** when stdout or stderr is over the limit (keep 50 KB for now,
  and consider lowering it to about 16 KB after measuring), write the full
  stream to `<session-tmp>/tool-output/<callID>.<stdout|stderr>.log`, and
  return:
  ```
  stdout: 812 KB, 9,431 lines, full output saved to /…/call_abc.stdout.log
  --- last 80 lines ---
  …
  ```
  The model can then `grep`, or use `read_file` with a line range, on the
  saved file. Keep the tail, not the head.
- Do the same for `firecrawl.go` / `webFetch` and for `bashJob` output
  pages.
- **Test:** `tests/tools/spill_test.go`. Check large output gives a file
  with the full contents, a tail, and the stated size; small output is
  unchanged.

### Task 3.2: Strip noise from command output
- **Change:** in bash results, remove ANSI escape codes and collapse
  carriage-return progress bars (keep only the last frame of each `\r`
  sequence). This is just a transform before the text reaches the model.
  The spill file keeps the raw bytes.
- **Test:** add to the spill test file.

### Task 3.3: Fix recurring tool errors
- Using the Task 1.3 stats from the baseline, fix the top 3 error causes
  (for example, `editFile` "must match exactly once" failures: return the
  closest candidate lines; invalid-argument messages: name the bad field).
  Each fix is its own commit with its own test.

---

## Phase 4: Behind flags (A/B with bench)

Every task in this phase adds a flag read from `console.toml`
`[harness]` or from the environment (`CONSOLE_HARNESS_<NAME>=1`), passed
through `BuildOptions` and the tool setup. The default stays at today's
behavior until bench shows no drop in success and lower cost. Then flip the
default in a separate commit.

Put flag loading in a new `internal/agent/flags` package with a `Flags`
struct. `bench --flags` sets it directly.

### Task 4.1: `prompt_trim`: rewrite the system prompt
Label every line: **keep / rewrite / delete / move**. Planned changes:

| Line / section | Action | Reason |
|---|---|---|
| `defaultIdentity` | keep, tighten | Product identity. Drop "correctly and concisely" (asks for brevity). |
| `# Tool inventory` list | **delete** | Repeats the tool schemas. |
| "You MUST follow the context files below for all tasks:" | rewrite | → "Project rules from AGENTS.md and similar files:" |
| "Context files above are loaded automatically. Do not waste tools grepping for AGENTS.md…" | rewrite | → "These files are already loaded." (a quirk we've seen, so keep it as a fact) |
| `<critical>` "Each response MUST advance the task." | delete | Capable models do this already; emphasis. |
| `<critical>` "Default to informed action; do not ask for confirmation…" | rewrite, keep | Real quirk (stopping to ask permission). → "When tools or the repo can answer a question, use them instead of asking the user." |
| Plan-mode `<critical>` | keep, rewrite | The mode depends on it. → "Plan mode: discuss and ask questions; no file writes." |
| "Today is X. Working directory is Y." | delete | Repeats the workstation block (which moves to setup in Phase 2). |
| Skills header sentence | keep | Tells the model the skill lookup exists. |
| Approval-mode text (`approvalModeInstructions`) | audit, rewrite | Remove emphasis; keep what each mode allows. |
| Tool descriptions (`jsonschema` tags) | tighten | Describe behavior and arguments; remove usage lectures (for example, bash's "not for reading or editing files" → keep one short line, since that's a real quirk). |

Also audit `SYSTEM.md` handling and the subagent preamble in the same pass.
**Measure:** static tokens per request, success, turns.

### Task 4.2: `no_workspace_tree`
- Skip the workspace tree (`SkipWorkspaceTree` already exists; connect it to
  the flag). Keep OS, git branch and repo status (dirty/clean, as one line).
- **Watch:** turns per task and `list_dir`/`glob` calls on the
  code-question tasks. Kill the flag if turns go up enough to cancel out the
  savings.

### Task 4.3: `sparse_line_numbers`
- `read_file` numbers only line 1, every line that's a multiple of 10, and
  the last line, like this:
  ```
   1: package foo
      import "x"
  ...
  10: func Bar() {
  ```
  Keep the column width aligned so line counting stays easy.
- **Watch:** `editFile` failure rate and citation accuracy on the
  code-question tasks (the checks assert on the cited line numbers).

### Task 4.4: `lazy_tools`: load rarely used tools on demand
- **Always sent:** `read_file`, `list_dir`, `glob`, `grep`, `editFile`,
  `write_file`, `bash`, `todo`, `ask` (models call it even when it's
  missing), `subagent`, `readSkill`.
- **Loaded when needed:** `webSearch`, `webFetch`, `bashJob`, `askMany`,
  `memory`, `batchWrite`. Future MCP tools also go here, one group per
  server.
- **How:** add a `loadTools` tool (`{names: string[]}` or `{group}`), and a
  one-line list of the deferred tool names in the setup message ("More tools
  available through loadTools: webSearch, webFetch, …"). After a call, the
  loaded definitions are added to the tool list for **the rest of the run**,
  appended at the end so the earlier tool prefix stays cached. Status notes
  (for example, "firecrawl key missing") go on that list line.
- `bashJob` exception: when `bash` starts a background job, load `bashJob`
  automatically, so the model never calls a tool it can't see.
- **Files:** `run/turns.go` (tool list), `agent/loop/loop.go` (tool list
  can change during a run), new `tools/load_tools.go`.
- **Watch:** calls to unknown tools (count them in Task 1.3), turns, and
  cost. Try 2 configurations (the list above, and the list above with
  `webFetch` always sent).

### Task 4.5: `compaction_v2`
- Before compacting, save the full history to
  `<session-dir>/history-<n>.jsonl`, and put the path at the end of the
  summary ("Full earlier history: <path>. grep it for details.").
- Carry the todo list and the unfinished steps forward word for word
  (from the todo tool's state), not paraphrased.
- Keep the summary prompt short. Try the current prompt against a
  one-liner, and measure summary size and success after compaction on the
  multi-step tasks (force compaction with a low threshold in bench).
- **Files:** `agent/compaction/compact.go`, `summary.go`, `files.go`.

### Task 4.6: `subagent_lean`
- A subagent gets the **stable** system prompt plus a short setup (cwd, OS,
  branch). No tree and no parent-only sections.
- Replace "summarize your findings cleanly" with a return format: **Done /
  Findings / Concerns / Deviations**. Return only the child's **final**
  assistant message, not every streamed text delta.
- Keep the subagent tool description neutral. Don't push the model toward
  delegating. No model choice in the arguments (subagents always use the
  parent's model unless the harness says otherwise).
- **Files:** `agent/loop/subagent.go`, `tools/subagent_tool.go`.
- **Watch:** cost of the whole tree (parent plus subagents, from Task 1.1).

---

## Phase 5: Proposals only (don't implement without sign-off)

- **Model routing and reasoning effort:** send simple turns (short
  questions, no tool calls expected) to a cheaper model or lower
  `ThinkingLevel`. Needs real usage data first.
- **Vision fallback as a subagent** (Task 2.5).
- **Planner plus cheap workers** for multi-agent runs. Measure the whole
  tree; a cheaper planner can make the workers more expensive.
- **Semantic code search** alongside grep, to cut exploration turns.
- **Agent-maintained notes file** per project (line budget, rewritten rather
  than appended, loaded at start). This could build on the `memory` tool.
- **Pass back reasoning items:** check that each provider returns
  reasoning/thinking signatures on later turns (Claude thinking blocks,
  Codex encrypted reasoning, Gemini `ThoughtSignature`), and alert when
  they're missing. Move this to Phase 3 as a direct fix if the audit finds
  any being dropped.

---

## Validation and shipping rules

- For each Phase 2–4 change: `bench --runs 3` before and after, then
  `bench compare`.
- **Ship when** median weighted cost per successful task goes down **and**
  success rate, tool-error rate and median turns don't get worse by more
  than noise (roughly, one task out of 12 swinging either way).
- Record null and negative results in the Results log below. Don't delete
  them.
- Before flipping any Phase 4 default: one confirmation run on a paid model,
  if available.

## Traps to avoid

- Telling the model to use fewer tokens or do less.
- Cutting tool output off without saving the rest.
- Dropping reasoning items to save input tokens.
- Changing values, or a changing tool order, inside the cached prefix.
- Lazy-loading a tool the model needs on turn 1 or calls when it's missing.
- Emphasis-heavy prompts.
- Forcing terser output than the model is used to.
- Optimizing raw tokens instead of weighted cost, per request instead of
  per task.
- Switching models mid-conversation.

## Commit plan

One task per commit, each with a single-line message, for example:
`opencode: send system prompt on chat completions path`,
`agent: aggregate run usage across turns and subagents`,
`bench: add token-efficiency bench runner`,
`systemprompt: split stable prompt from per-session setup`, …

## Results log

| Date | Change | Runs | Success | Median turns | Weighted cost / success | Notes |
|---|---|---|---|---|---|---|
| — | baseline | — | — | — | — | fill in after Task 1.6 |
