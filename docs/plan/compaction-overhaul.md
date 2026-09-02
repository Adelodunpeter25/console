# Compaction Overhaul — Design & Implementation

## 1. Overview

Context window compaction is the safety net for unbounded agent runs. The current impl in `apps/server/agent/src/compaction/index.ts` is structurally fine but operationally broken: the "summary" is a metadata stub, the feature is off by default, and `keepRecentTurns: 4` discards most of the recent context window in real coding sessions. This plan ships three changes — implementation fixes (truncation + summary) **before** enabling the feature by default.

### Goals
- Compaction runs on every session by default (not opt-in).
- The "summary" is a real, useful recap — not a count of dropped messages.
- Big tool outputs are aggressively truncated **before** they reach the compaction decision, so the threshold math reflects what actually matters.
- The model after compaction still has the structure of what happened: which files were touched, which commands ran, which errors were hit.

### Non-goals
- No vector / semantic retrieval of old context. Compaction stays linear.
- No changes to providers or system-prompt builder. Compaction only touches `compaction/`, `agent-loop.ts`, `agent.ts`, `tool-executor.ts`, and the message type.
- No new event variant on the wire. The existing `compaction` event gains richer fields.

---

## 2. Current state (audit)

### 2.1 Existing code
- `compaction/index.ts:20-42` — `estimateMessageTokens()` uses `chars / 4`. Counts user prompt chars, assistant text/thinking/tool-arg JSON, and tool-result content JSON. No weighting for images or tool metadata.
- `compaction/index.ts:47-57` — `shouldCompact()` fires at 80% of `model.contextWindow` (or `tokenThreshold` if set).
- `compaction/index.ts:62-107` — `compactHistory()` splits at `messages.length - keepRecentTurns` (default **4**), drops the prefix, replaces it with a fake user+assistant pair whose assistant text is literally `"[Conversation Checkpoint: Compacted 12 prior messages (3 user, 5 assistant, 4 tool results).]"` — i.e. only metadata, no content.
- `service/agent-loop.ts:93-101` — calls `compactHistory()` when `compaction && shouldCompact(...)`. Emits a `compaction` event with `summary` and `originalMessageCount`.
- `service/agent.ts:176-186` — `Agent.run()` builds `AgentLoopConfig` but **never sets `compaction`**. The feature is dead unless a caller passes it explicitly.

### 2.2 What's wrong
1. **Default-off.** A user who never reads the source code will never get auto-compaction. Sessions blow past context window, get truncated by the provider, and look like the model "forgot" the task.
2. **Lossy summary.** The model in the next turn sees a line that says "12 messages were compacted" but no idea what they were about. It re-derives the same exploration, hits the same files, re-asks the same questions.
3. **`keepRecentTurns: 4` is too small.** A "turn" in this codebase is one model response. 4 responses × tool output is roughly 8-20 messages. Real coding sessions blow past 4 within the first 60 seconds of work.
4. **No truncation before estimation.** A 200k-token `npm test` log is one `toolResult.results[0].content`. It inflates `estimateMessageTokens` by 50k and triggers compaction 4 turns too early.

---

## 3. Change #1 — Default compaction on, larger keep window

The smallest, lowest-risk change. Touches two lines in `agent.ts` and updates the default in `compaction/index.ts`.

### 3.1 Behavior
- `AgentOptions` gains no new field. `Agent` constructor always passes a `compaction` config to `runAgentLoop` unless one is explicitly disabled.
- Default `keepRecentTurns` raised from `4` → `12`.
- A caller can still opt out by passing `compaction: false` (or a new `compaction: undefined` after we tighten the type — see 3.3).

### 3.2 File changes
- `apps/server/agent/src/compaction/index.ts`
  - Update the default in the JSDoc on `CompactionOptions.keepRecentTurns` (line 11): `Default: 12`.
  - Update the default in `compactHistory()` (line 66): `?? 12`.
- `apps/server/agent/src/service/agent.ts`
  - In `run()` where the `config: AgentLoopConfig` is built (lines 176-186), always set:
    ```ts
    compaction: options.compaction ?? {
      maxThresholdRatio: 0.8,
      keepRecentTurns: 12,
    },
    ```
  - Where `options` here is the run-time options (we may need a small refactor to thread a `compaction` override through `Agent.run()`'s params — see 3.3).

### 3.3 Open question (resolved here)
Today `AgentOptions` has no `compaction` field. We need one. Decision: add `compaction?: CompactionOptions | false` to `AgentOptions`; `false` means "off", `undefined` means "use defaults". `run()` resolves the user's value to the final config.

### 3.4 Tests
- New `apps/server/tests/compaction-defaults.test.ts`:
  - Construct an `Agent` with no `compaction` option. Inspect the resolved `AgentLoopConfig.compaction` after calling `agent.run(...)` (or extract a small `buildRunConfig()` helper to test directly).
  - Assert it equals `{ maxThresholdRatio: 0.8, keepRecentTurns: 12 }`.
  - Construct with `compaction: false`. Assert it's omitted from the config.
  - Construct with `compaction: { keepRecentTurns: 3 }`. Assert the override is preserved.

---

## 4. Change #2 — Real summary, not a metadata stub

Replace the placeholder summary string with a structured recap generated from the dropped messages. Two layers:

### 4.1 Layer A — Structural recap (zero model calls, free)

For each dropped message, extract:
- `user` → the prompt text (truncated to first 500 chars).
- `assistant` → tool call names + argument keys (never argument values); text parts truncated to first 200 chars; thinking parts dropped.
- `toolResult` → tool names + the **first 500 chars** of each result; a one-line tag like `[ok]` / `[err]` based on `isError`.

Compile this into a deterministic format:

```
[Conversation Checkpoint — 12 messages compacted at 14:32]

User asked: "Refactor the auth middleware to use the new token store."
Assistant called: read_file({"path": "src/auth/middleware.ts"}), read_file({"path": "src/auth/store.ts"})
Tool read_file (src/auth/middleware.ts): 487 chars [ok]
  > import { tokenStore } from './store'...
Assistant called: edit_file(...)
Tool edit_file (src/auth/middleware.ts): 23 chars [ok]
User asked: "Now add rate limiting."
Assistant called: write_file({"path": "src/middleware/rate-limit.ts"})
Tool write_file: 1,204 chars [ok]
  > import { RequestHandler } from 'express'...
... 6 more turns
```

This is the **default** summary strategy. Free, deterministic, and gives the model enough breadcrumbs to continue without re-deriving.

### 4.2 Layer B — Optional LLM-generated prose (opt-in, costs a model call)

Add a new option `compaction.summary: "structural" | "llm"`. Default `"structural"`. When `"llm"`:
- After structural recap is built, call the model's `streamFn` with a tiny prompt: "Summarize the above conversation checkpoint in 3-5 sentences for your future self."
- Stream the result; replace the structural block's preamble with the LLM's text.
- Keep the structural detail below the LLM prose so the model still has the data.
- On LLM failure or abort, fall back to the structural recap silently.

Costs an extra model call per compaction event. For a 1M-token run with 5 compactions, that's 5 extra calls. Opt-in is the right default.

### 4.3 File changes
- `apps/server/agent/src/compaction/index.ts`
  - New internal function `buildStructuralSummary(messages: AgentMessage[]): string`.
  - New internal function `buildLlmSummary(structural: string, streamFn, model, signal): Promise<string>`.
  - `compactHistory()` calls the configured strategy. Signature changes to optionally take `{ streamFn, model, signal, strategy }`.
- `apps/server/agent/src/compaction/types.ts` (new) — re-export `SummaryStrategy = "structural" | "llm"`.
- `apps/server/agent/src/compaction/index.ts` — extend `CompactionOptions`:
  ```ts
  summary?: SummaryStrategy;            // default "structural"
  streamFn?: StreamFn;                  // required when summary === "llm"
  model?: Model;                        // required when summary === "llm"
  signal?: AbortSignal;                 // propagated into the LLM call
  ```
- `apps/server/agent/src/service/agent-loop.ts` — pass `streamFn` and `model` into `compactHistory` when calling it.
- `apps/server/agent/src/service/agent.ts` — pass `streamFn` into the resolved `compaction` config so Layer B can use it.
- `apps/server/agent/src/utils/text-truncate.ts` (new) — shared `truncate(s, n)` used by both the structural summarizer and the tool-result truncator (Change #3).

### 4.4 The `compaction` event gets richer

Currently:
```ts
{ type: "compaction", summary: string, originalMessageCount: number }
```

Becomes:
```ts
{
  type: "compaction",
  summary: string,             // the recap text (now actually useful)
  strategy: "structural" | "llm",
  originalMessageCount: number,
  compactedMessageCount: number,
  droppedTokens: number,       // how many tokens we shed (before → after)
}
```

Additive — no client breakage. The new fields are optional in the type.

### 4.5 Tests
- `apps/server/tests/compaction-summary.test.ts` (new):
  - **Structural recap correctness.** Build a synthetic 20-message history. Run `compactHistory()`. Assert the summary mentions every tool name, every file path, contains no tool result body beyond 500 chars, no thinking parts.
  - **No-op when short.** A 5-message history returns the original messages verbatim.
  - **LLM fallback.** Mock `streamFn` returns text. Run with `summary: "llm"`. Assert the LLM text appears and the structural preamble appears below it.
  - **LLM failure fallback.** Mock `streamFn` throws. Assert the structural recap is still returned and the function does not throw.

---

## 5. Change #3 — Truncate tool results before they hit the context window

The single biggest source of context bloat in a coding agent is tool output. A `bash` that runs `npm test` or a `read_file` on a 5000-line file both produce massive `toolResult` content. Today, this content sits in the message array verbatim until the next compaction, inflating `estimateMessageTokens` and forcing premature compactions.

### 5.1 Behavior
- A new function `truncateToolResults(messages, options)` runs **before** `shouldCompact()` is called. It returns a new array where each `toolResult.results[*].content` is capped at a configurable character budget.
- Default per-result cap: `8000` chars (~2000 tokens). Head + tail kept with a `[truncated N chars]` marker in the middle.
- Truncation is **lossy** but **reversible on the file system** — the model can always re-`read_file` if it needs the missing region.
- The truncation budget is configurable: `compaction.maxToolResultChars` (default 8000). `0` disables.

### 5.2 Why this matters
- `estimateMessageTokens` becomes a much better signal. A 200k-char `npm test` log no longer counts as 50k tokens — it counts as 2000.
- Compaction triggers at the right time, not when one big tool result blows the budget.
- Sessions with many large reads survive far longer before compaction.

### 5.3 File changes
- `apps/server/agent/src/utils/text-truncate.ts` (new) — `truncate(s: string, max: number): string`. Keep head + tail with a marker line.
- `apps/server/agent/src/compaction/index.ts`
  - New exported function `truncateToolResults(messages, { maxChars }): AgentMessage[]`.
  - Extend `CompactionOptions` with `maxToolResultChars?: number` (default 8000).
  - `shouldCompact` is replaced by `shouldCompact(messages, model, options)` that internally calls `truncateToolResults` first and operates on the truncated view. The loop in `agent-loop.ts` is unchanged.
- `apps/server/agent/src/service/agent.ts` — `compaction` config now includes `maxToolResultChars: 8000` by default.

### 5.4 Tests
- `apps/server/tests/compaction-truncation.test.ts` (new):
  - **Big tool result is truncated.** A `toolResult` with 50,000 chars. After `truncateToolResults`, each result is ≤ 8000 chars and contains head + tail + `[truncated …]` marker.
  - **Small tool results are untouched.** A 100-char result comes through unchanged.
  - **`maxToolResultChars: 0` disables.** All results pass through untouched.
  - **Estimation is honest.** After truncation, `estimateMessageTokens` returns a value lower by the expected amount.
  - **Compaction is delayed.** A history that triggers compaction before truncation does **not** trigger it after truncation.

---

## 6. Rollout order — implementation before enable

Compaction must be **correct before it is on by default**. We fix the implementation (truncation + summary) first, then flip the default.

| Order | Change | Risk | Lines of code (est.) | Notes |
|---|---|---|---|---|
| 1 | #3 truncation | Low | ~80 | Pure function, easy to test, no schema changes. Fixes `estimateMessageTokens` so threshold math is honest before anything else. |
| 2 | #2 structural summary | Medium | ~150 | Core fix: replaces metadata stub with real recap. Makes every future compaction useful. No event schema break. |
| 3 | #2 LLM summary | Higher | ~120 | Optional `summary: "llm"` — costs a model call; needs abort + timeout handling. Ship after structural is stable. |
| 4 | #1 default-on | Very low | ~10 | Last step. Now safe to ship: `keepRecentTurns: 12`, `compaction` on by default, `false` to opt-out. |

Ship 1 → 2 together in one PR (both are pure implementation fixes). Ship 3 separately. Ship 4 separately once 1-2 are verified in prod.

---

## 7. Event schema additions

The wire format only **adds** fields. Old clients ignore them.

```ts
// types/events.ts — additive extension
export type CompactionEvent = {
  type: "compaction";
  summary: string;
  strategy: "structural" | "llm";        // new
  originalMessageCount: number;          // existing
  compactedMessageCount: number;        // new
  droppedTokens: number;                 // new
};
```

No breaking changes. The `AgentSessionEvent` union gains no new variant.

---

## 8. Test plan summary

| Test file | Covers |
|---|---|
| `compaction-defaults.test.ts` (new) | #1 — defaults are on, opt-out works, overrides preserved |
| `compaction-truncation.test.ts` (new) | #3 — tool result truncation, estimation impact, opt-out |
| `compaction-summary.test.ts` (new) | #2 — structural recap correctness, LLM fallback on failure |
| `agent-loop.test.ts` (extend) | End-to-end: a long session with 1+ compactions completes without `error` events |
| `permissions.test.ts` (unchanged) | Confirm permission flow is unaffected |

The three new test files use mock `StreamFn` only — no real LLM calls. They follow the existing pattern in `agent-loop.test.ts`.

---

## 9. File changes summary

| File | Change |
|---|---|
| `apps/server/agent/src/compaction/index.ts` | Major: defaults, `truncateToolResults`, `buildStructuralSummary`, optional LLM summary, richer return type |
| `apps/server/agent/src/compaction/types.ts` (new) | `SummaryStrategy` type |
| `apps/server/agent/src/service/types.ts` | No change (compaction config is just richer) |
| `apps/server/agent/src/service/agent.ts` | Default-on `compaction`; pass `streamFn` for LLM summary |
| `apps/server/agent/src/service/agent-loop.ts` | Call `truncateToolResults` before `shouldCompact`; pass `streamFn` to `compactHistory`; richer `compaction` event |
| `apps/server/agent/src/utils/text-truncate.ts` (new) | `truncate(s, max)` helper |
| `packages/types/src/events.ts` | Extend `compaction` event with `strategy`, `compactedMessageCount`, `droppedTokens` |
| `apps/server/tests/compaction-defaults.test.ts` (new) | #1 tests |
| `apps/server/tests/compaction-truncation.test.ts` (new) | #3 tests |
| `apps/server/tests/compaction-summary.test.ts` (new) | #2 tests |

---

## 10. Out of scope (deferred)

These are real ideas but not part of this plan:

- **Vector / semantic retrieval of past context.** Useful for cross-session memory but the gain is unclear for within-session compaction.
- **Compaction over images.** `ImagePart.attachments` is counted as raw bytes today. Real vision tokens are ~765 per image. Out of scope.
- **Cross-session memory.** Different problem — `roadmap.md` covers it under "Cross-chat memory".
- **User-visible "Compact now" button.** Pairs nicely with these changes but is a UI feature, not an engine one.
