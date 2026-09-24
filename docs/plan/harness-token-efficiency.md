# Harness Token Efficiency

Status: **in progress** (paused 2026-09-24). Phase 1 done, Task 3.0 done,
`sparse_line_numbers` measured once. See "Next steps" under Results log.

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
| Usage telemetry | `loop.TurnUsage` (`agent/loop/message.go:66`), each provider's `NormalizeUsage` | Per-turn input, cache read/write and output are recorded on the assistant message, but never added up per task, per tool, or per context source. `run/turns.go:380` swallows `EventUsage`. |
| File reads | `tools/file_tools.go:120–123` | Every line is numbered (`%*d: `). |
| Bash output | `tools/bash_tools.go:17,30–34` | Keeps the first 50 KB and drops the rest. Errors usually show up at the **end**, which is the part that gets thrown away. |
| Firecrawl / webFetch | `tools/firecrawl.go:22–29` | Also cuts off the head without saving the rest. |
| Tools shipped | `tools.DefaultTools()` + memory | 17 tools with full schemas on every request (`webSearch`, `webFetch`, `bashJob`, `askMany`, `memory`, `batchWrite` are rarely used). OpenCode adds 6 compatibility marker tools on top. |
| Compaction | `agent/compaction/llm.go:24` | Summary prompt is already short. The full history isn't saved anywhere the agent can search. |
| Subagent | `agent/loop/subagent.go:129` | Adds a fixed preamble to the parent's **whole** system prompt (tree included). Returns the child's entire streamed text as its "summary". |

## Test setup: Claude Haiku on the Claude Code provider

All cost measurements use the `claude` provider (Claude Code OAuth
subscription) with **`claude-haiku-4-5`**. That's the model with remaining
usage on the account.

Why Haiku instead of the OpenCode free model:

- **Real cache numbers.** Anthropic reports `cache_read_input_tokens` and
  `cache_creation_input_tokens` on every response, and `NormalizeUsage`
  (`providers/claude/stream.go:228`) already maps them into `TurnUsage`.
  We don't need to estimate caching locally.
- **Explicit cache breakpoints** are already wired up
  (`claude/convert.go:251,316`, `stream.go:150`), so the Phase 2 work is
  measured on the provider it's actually designed for.
- **Real prices.** Haiku 4.5 list prices (per million tokens) are uncached
  input $1.00, cache read $0.10, cache write (5 min) $1.25, output $5.00.
  The bench reports weighted cost in real dollars using these prices, kept
  as a table in the bench tool so it's easy to add other models.
- More consistent quality than free models, so each run is less noisy.

Things to know about Haiku and the subscription:

- **Minimum cacheable prefix.** Haiku 4.5 only caches a prefix of at least
  **4,096 tokens**. A cache breakpoint on a shorter prefix gets no cache
  hit, even when the prefix is identical. That matters after Phase 4.1
  trims the prompt: the tools plus stable system block may drop under
  4,096 tokens. If so, the breakpoint after the setup message (tools +
  system + setup) is the one that actually caches. Task 2.3 has to check
  this with real numbers instead of assuming.
- **The usage limit must not be exceeded.** The subscription limit is a
  hard ceiling, so the bench has a budget guard (Task 1.7) that checks the
  Claude usage report before and during a run, and stops early.
- **Keep runs small.** Default to `--runs 2` on the full 12-task set, and
  run only the tasks a change affects when checking just that change. Save
  the full set for the baseline and for final checks before flipping a
  flag's default.
- **Rate limits.** `doRequest` already retries on 429 and 529. The bench
  runs tasks one after another with a short pause between them.
- **Required identity line.** `ClaudeCodeSystemInstruction` stays as the
  first system block. Prompt trimming (4.1) never touches it.

OpenCode's free model (`space-bunny-free`) is an optional **second check**,
useful to see whether prompt changes still work on a weaker, non-Anthropic
model. Zen does report some cached tokens on the Chat path. If we want better
cache numbers there, sending `prompt_cache_key` on the Chat path (as the
Responses path already does) is a small optional follow-up.

---

## Phase 1: Telemetry and baseline (change directly)

Everything after this depends on it.

### Task 1.1: Token totals per run
- **Files:** `internal/agent/loop/loop.go`, new `internal/agent/loop/usage.go`.
- **Change:** keep a `RunUsage` on the agent that adds up every
  `TurnUsage` in a run: turns, input, cacheRead, cacheWrite, output,
  reasoning, plus dollar cost from the bench price table. At the end of a run, emit it
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
- **Runner:** for each task, repeated N times (default 2): reset the
  worktree, run the agent loop in-process with the `claude` provider and
  `claude-haiku-4-5` (using the same credential the app uses,
  `claude.LoadCredential`), run `check`, and record success, turns,
  RunUsage, dollar cost, cache hit rate, tool stats and wall time.
- **Flags:** `--provider` (default `claude`), `--model` (default
  `claude-haiku-4-5`), `--runs`, `--tasks`, `--flags key=value,...` (turns
  on harness feature flags, see Phase 4), `--max-usage-pct` (see Task 1.7),
  `--out results.json`.
- **Output:** a table per task and overall: success rate, median turns,
  median dollar cost per **successful** task, cache-read share of input,
  cold-miss rate (turns after the first with `cacheRead == 0`), tool error
  rate, and cost share by source. Also `bench compare a.json b.json` to show
  the difference.
- **Test:** `tests/bench/bench_test.go` runs the runner with a fake provider
  (no network).

### Task 1.6: Record the baseline
- Run `go run ./cmd/bench --runs 2 --out bench/results/baseline.json`
  (Claude Haiku).
- Add a short "Baseline" section to this doc: cost share by source × billing
  type, static tokens per request, turns per task, and per-tool usage and
  error rate. Rank the rest of the work with it.

### Task 1.7: Usage budget guard for the Claude subscription
- **Files:** `cmd/bench/budget.go`, reusing `providers/usage`
  (`Service.GetUsage(ctx, "claude")` → `Report.Limits[].Amount`, which is a
  utilization percentage).
- **Change:**
  - Before starting, fetch the Claude usage report. Refuse to start if any
    limit (5-hour session, weekly, weekly per-model) is at or above
    `--max-usage-pct` (off by default; pass e.g. `--max-usage-pct 80` to enable).
  - Re-check between tasks (call `Service.Invalidate("claude")` first, so
    the numbers aren't cached). Stop cleanly as soon as the limit is
    crossed, and write partial results marked `"aborted": "budget"`.
  - Estimate how much a run will use before it starts: tasks × runs × the
    median cost per task from the last results file. Print the estimate,
    and ask for `--yes` when it's more than 10 percentage points of the
    remaining budget.
  - Also stop immediately on a 429 that has already used up its retries,
    since that means the limit is hit.
- **Test:** `tests/bench/budget_test.go`, with a fake usage report: refuses
  above the threshold, stops partway through, and writes partial results.

---

## Baseline (2026-09-24, Claude Haiku 4.5, commit 708a7b21)

`apps/server-go/bench/results/baseline.json`: 12 tasks × 2 runs, $1.24 total.

| Metric | Value |
| --- | --- |
| Success rate | 67% (16/24) |
| Median turns per task | 14 |
| Median cost per successful task | $0.045 |
| Share of input read from cache | 94% |
| Cold misses (later turns with no cache read) | 0% |
| Tool error rate | 12% |

Estimated input tokens by source, summed over every request:

| Source | Share |
| --- | --- |
| Tool definitions | 32.0% |
| `read_file` results | 26.7% |
| `bash` results | 9.2% |
| Tool-call arguments in history | 8.7% |
| `grep` results | 3.9% |
| System prompt (all sections) | 14.4% (repo-rules 3.9, workspace tree 3.7, skills 3.0, commands 1.4, other 2.4) |
| Assistant text | 2.7% |

What it tells us:
- Tool definitions are the largest single cost, and they are sent on every
  turn. Trimming them (Phase 3/4) is the biggest win.
- `read_file` output is next: whole files are read and kept in history.
  Output limits and read ranges (Phase 3) come second.
- The cache already works well (94%, no cold misses). Phase 2 aims to cut
  what gets written to the cache, not to fix misses.
- The 8 failures come from the model, not the harness: Haiku answers without
  searching (task 02), and edits the old TypeScript server `apps/server`
  instead of `apps/server-go` (tasks 03, 09). Keep these tasks; a harness
  change should not make them worse.
- The same task can take 15 or 39 turns, so compare runs by median, with
  at least 2 runs per task.

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
- **Haiku minimum:** caching only works from 4,096 tokens up. Log the token
  count at each breakpoint, and when it's under 4,096, skip that breakpoint
  (it's wasted). The bench confirms the first turn shows a
  `cache_creation_input_tokens` write, and later turns show
  `cache_read_input_tokens` for tools + system + setup.
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
Run bench (Haiku) against the baseline. Expect: the same success rate, a
higher cache-read share of input from turn 2 onward, and fewer cold misses
(turns after the first with `cacheRead == 0`). All of these come from real
Anthropic usage fields.

---

## Phase 3: Tool output (change directly)

**Order change (2026-09-24):** Phase 2 Tasks 2.3–2.5 are paused. `read_file`
results (26.7%) are handled before bash output (9.2%).

### Task 3.0: Smaller default reads (done)
- `read_file` with no `startLine`/`endLine` returns the first 300 lines, with
  a header like "Output truncated at line 300 of 842. Continue with
  startLine=301." Ranged reads still go up to the 2,000-line ceiling.
- Tool descriptions point to our own search tools (`grep`/`glob`), never to
  searching through the shell.
- Sparse line numbers (Task 4.3) exist behind
  `CONSOLE_HARNESS_SPARSE_LINE_NUMBERS=1`, off by default. When it's on, the
  desktop read-file view shows blanks for unnumbered lines.
- Replacing outdated copies of a re-read file in history is **deferred**. It
  rewrites older messages, which breaks the cache from that point on.
- **Validate:** one bench run against the baseline. Watch success rate, turns
  per task, and the `read_file` share. If turns go up a lot, raise the cap.
- **Test:** `tests/tools/read_file_test.go`.

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

- For each Phase 2–4 change: `bench --runs 2` (Haiku) on the affected tasks
  before and after, then `bench compare`. Keep the budget guard on.
- **Ship when** median dollar cost per successful task goes down **and**
  success rate, tool-error rate and median turns don't get worse by more
  than noise (roughly, one task out of 12 swinging either way).
- Record null and negative results in the Results log below. Don't delete
  them.
- Before flipping any Phase 4 default: one run of the full 12-task set on
  Haiku. Optionally, a second check on the OpenCode free model to see that
  the change also works on a non-Anthropic model.

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

| Date | Change | Runs | Success | Median turns | $ cost / success (Haiku) | Notes |
|---|---|---|---|---|---|---|
| 2026-09-24 | baseline (708a7b21) | 24 | 16/24 | 14 | $0.077 | subagent calls all failed on this commit |
| 2026-09-24 | read cap 300 lines (bca9d973) | 24 | 19/24 | 15.5 | $0.085 | more reads and turns; different code than baseline |
| 2026-09-24 | `sparse_line_numbers` (6b064876) | 24 | 19/24 | 13.5 | $0.091 | read output ~19% smaller per call; one runaway run (06 run 2: 69 turns, $0.36) skews cost; subagent fixed |

### Next steps (resume here)
- The three rows above ran on different commits and only 2 runs per task,
  so run-to-run noise (often 2x per task) is bigger than the effects. They
  are not a fair A/B.
- Redo the A/B on one commit with `--runs 3`: flag off vs
  `CONSOLE_HARNESS_SPARSE_LINE_NUMBERS=1`, using `--prev` to compare.
- Env-var flags are not recorded in the results `flags` field; consider
  saving active `CONSOLE_HARNESS_*` vars in the results file.
- Then continue with Phase 2 (cache layout).
