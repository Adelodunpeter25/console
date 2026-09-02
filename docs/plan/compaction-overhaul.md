# Compaction Overhaul — Design & Implementation

## 1. Overview

Context window compaction is the safety net for unbounded agent runs. In a coding assistant, sessions routinely accumulate hundreds of thousands of tokens across iterative edits, large file reads, compiler errors, test logs, and multi-turn refactors.

The current implementation in `apps/server/agent/src/compaction/index.ts` has several critical limitations:
1. **Default-off**: The compaction config is never passed by `Agent.run()`, leaving the engine dead unless manually enabled in code.
2. **Metadata stub summary**: Discarded messages are replaced with an empty count: `"[Conversation Checkpoint: Compacted 12 prior messages...]"`, causing the model to lose all context of past work, re-read previously inspected files, and repeat mistakes.
3. **Turn-unsafe slicing**: Slicing by raw index (`messages.length - keepRecentTurns`) splits right between an `assistant` tool-call and its `toolResult`, or between adjacent assistant turns, causing immediate **HTTP 400 Bad Request** errors from Anthropic, Gemini, and OpenAI providers.
4. **No tool output truncation in context**: A 200k-char `npm test` log or 5,000-line file read sits verbatim in `messages`, artificially blowing past token limits turns too early.
5. **Disconnected persistence**: Compaction inside `agent-loop` mutates an internal array, but `Agent._messages`, `sessionStorage`, and UI state are never updated. On the next user prompt, the uncompacted history is reloaded from disk and immediately re-triggers compaction.

This plan overhauls compaction in `console`, drawing direct inspiration from the production-tested compaction architecture in **`oh-my-pi`** (`docs/compaction.md` and `packages/agent/src/compaction/`).

---

### Goals
- **Turn-safe boundary selection**: Never slice at a `toolResult` or orphan tool calls; preserve provider role alternation invariants.
- **In-place tool result truncation**: Cap oversized tool outputs (head + tail with elision marker) so single noisy tools don't consume the context window.
- **High-signal summary**: Generate a structured recap containing the original user task, decisions, errors encountered, and a cumulative file operations tree (`<files>` with `Read`, `Write`, and `RW` markers).
- **Default-on with token-based budgeting**: Default to `keepRecentTokens: 20_000` (or ~15-20% context window reserve) rather than arbitrary message counts.
- **End-to-end persistence**: Sync compacted history to `sessionStorage` and `Agent._messages`, and broadcast rich `compaction` events to mobile and desktop clients.

### Non-goals
- Vector / semantic retrieval of old context (linear compaction remains the source of truth).
- Native snapshot bitmap rasterization (`snapcompact` PNG frames); we keep text-based context summaries.
- Altering the system prompt builder or provider wire drivers.

---

## 2. Architectural Audit & Lessons from `oh-my-pi`

In `oh-my-pi`, compaction is structured around three core invariants:

```text
Before compaction:
  entry:   0     1     2     3      4     5     6      7      8     9
         ┌─────┬─────┬─────┬──────┬─────┬─────┬──────┬──────┬─────┬──────┐
         │ hdr │ usr │ ass │ tool │ usr │ ass │ tool │ tool │ ass │ tool │
         └─────┴─────┴─────┴──────┴─────┴─────┴──────┴──────┴─────┴──────┘
                 └────────┬───────┘ └──────────────┬──────────────┘
                messagesToSummarize            kept messages
                                    ↑
                           firstKeptEntryIndex (Turn boundary: never a toolResult)

What the LLM sees after compaction:
         ┌────────┬────────────────────────────┬─────┬─────┬──────┬──────┬─────┬──────┐
         │ system │ user: <summary> + <files>  │ usr │ ass │ tool │ tool │ ass │ tool │
         └────────┴────────────────────────────┴─────┴─────┴──────┴──────┴─────┴──────┘
              ↑                 ↑              └─────────────────┬────────────────┘
           prompt       compaction checkpoint       kept messages from cut point
```

### Invariants adopted for `console`:
1. **Valid Cut Points**: A cut point can ONLY occur at a `user` turn or an `assistant` turn whose tool results are fully contained in the kept window. It must **never** cut at a `toolResult`.
2. **File Operations Tracking**: Cumulative tracking across discarded turns (`read_file`, `edit_file`, `write_file`, `batch_write`) builds a folded directory tree with `(Read)`, `(Write)`, and `(RW)` markers.
3. **Prompt Cache Protection**: The summary is wrapped in a dedicated prompt template:
   ```markdown
   Prior model work/tool state available.
   MUST build on prior work; NEVER duplicate prior work.

   <summary>
   {{summary}}
   </summary>

   <files>
   {{files}}
   </files>
   ```
4. **Synchronized Storage**: Persisting the compaction replaces or marks compacted entries in `sessionStorage` so subsequent turns don't reload discarded messages.

---

## 3. Core Technical Specifications

### 3.1 Turn-Safe Cut-Point Selection (`findCutPoint`)

Instead of slicing by raw message count (`messages.length - 4`), implement `findCutPoint` in `apps/server/agent/src/compaction/cut-point.ts`:

1. **Calculate Token Budget**:
   - `keepRecentTokens` default: `20_000` tokens (or 20% of `model.contextWindow`, whichever is larger).
2. **Find Valid Cut Points**:
   - Scan `messages` from index 0 to `messages.length - 1`.
   - A message is a candidate cut point if:
     - `role === "user"`
     - `role === "assistant"` (only if it has no tool calls, or if its tool calls are followed by toolResults that stay with it).
     - **NEVER** `role === "toolResult"`.
3. **Accumulate From Newest to Oldest**:
   - Walk backwards from `messages.length - 1`, summing estimated tokens.
   - Once accumulated tokens exceed `keepRecentTokens`, select the closest valid cut point `cutIndex <= i`.
   - If no valid cut point exists, fall back to the earliest user message in the session.
4. **Boundary Integrity Check**:
   - Verify that no kept `toolResult` has its parent `toolCall` in the discarded partition.
   - If an assistant message with `toolCall` is kept, all its matching `toolResult`s must be in the kept partition.

---

### 3.2 In-Place Tool Result Truncation (`truncateToolResults`)

Massive tool results (e.g. `npm test`, `git log`, `read_file` on large binaries/dumps) must be bounded **in the active message list**:

1. **Per-Result Char Budget**:
   - Default: `8_000` chars (~2,000 tokens) per tool result item.
   - Configurable via `compaction.maxToolResultChars` (default `8000`, `0` to disable).
2. **Head + Tail Preservation**:
   - Keep `4_000` chars from the start (captures initial output/status/file headers).
   - Keep `4_000` chars from the end (captures test summaries, final errors, stack traces).
   - Insert placeholder in the middle:
     ```text
     \n\n[... Tool output truncated: 142,500 characters elided ...]\n\n
     ```
3. **Execution Point**:
   - Applied in `agent-loop.ts` when a `toolExecutionResult` arrives, before pushing to `messages`.
   - Applied retroactively to any uncompacted messages when initializing `runAgentLoop`.

---

### 3.3 High-Signal Summary Generation

Compaction replaces discarded messages with a rich checkpoint.

#### Layer A — Structural Summary + Cumulative File Operations (Default, Free, Fast)
Constructed deterministically without any extra LLM calls:
1. **User Prompt History**:
   - Truncated extract of each user prompt in the discarded window.
2. **Tool Invocations & Outcomes**:
   - Command names and arguments: `bash(git status)`, `read_file(src/index.ts)`.
   - Result status: `[ok]` or `[err: exit code 1]`.
3. **File Operation Tracking (`<files>`)**:
   - Tracks all touched paths in the discarded window.
   - Deduplicates and strips line selectors (`:1-50`).
   - Categorizes each file:
     - `(Read)` — read but never modified.
     - `(Write)` — written/created without prior read.
     - `(RW)` — read and subsequently modified.
   - Formats into a folded prefix tree capped at 25 files.

Example structural summary output:
```markdown
Prior model work/tool state available.
MUST build on prior work; NEVER duplicate prior work.

<summary>
[Session Checkpoint: 18 turns compacted at 15:42]

Initial Task: "Refactor session storage to use per-project SQLite databases and migrate tests."

Chronological Highlights:
- User requested SQLite migration and storage interface overhaul.
- Read existing memory and JSON storage implementations.
- Created `apps/server/agent/src/session/storage.ts` and `session-messages.ts`.
- Ran `bun test tests/session-storage.test.ts` [err: table missing columns].
- Modified schema migrations in `schema.ts` and re-tested [ok].
</summary>

<files>
# apps/server/agent/src/session/
schema.ts (RW)
session-messages.ts (Write)
storage.ts (Write)
utils.ts (Read)
# tests/
session-storage.test.ts (RW)
</files>
```

#### Layer B — Optional LLM Prose Summary (Opt-in)
When `compaction.summaryStrategy === "llm"`:
- Calls `streamFn` with a compact prompt:
  ```text
  You are an AI assistant compacting your previous context. Summarize the conversation so far into 3-5 concise bullet points highlighting:
  1. The core user request.
  2. What has been completed.
  3. Key architectural/code decisions.
  4. Known issues or next steps.
  ```
- Uses `temperature: 0` and max tokens `500`.
- On timeout, abort, or error, falls back instantly to Layer A structural summary.

---

### 3.4 Context Window Invariant & Role Alternation

When the summary replaces discarded turns:
1. Create `summaryUserMessage: AgentMessage`:
   ```ts
   {
     role: "user",
     content: renderCompactionSummaryContext(summaryText),
   }
   ```
2. Create `summaryAssistantMessage: AgentMessage`:
   ```ts
   {
     role: "assistant",
     id: randomUUID(),
     content: [{ type: "text", text: "Acknowledged. I have the context of prior work and touched files. Ready to proceed." }],
     stopReason: "stop",
   }
   ```
3. Because `firstKeptEntryIndex` is guaranteed to be a `user` turn:
   - The stream format will be:
     `[summaryUserMessage, summaryAssistantMessage, keptUserMessage, keptAssistantMessage, ...]`
   - This satisfies strict provider role alternation (User → Model → User → Model) for Anthropic, Gemini, and OpenAI without any consecutive duplicate roles.

---

### 3.5 Persistence & State Synchronization

In `apps/server/api/src/services/run.service.ts`:
1. When `event.type === "compaction"` is received:
   - Call `this.sessionStorage.replaceMessages(sessionId, event.compactedMessages)`.
   - Update `agent.loadHistory(event.compactedMessages)` so the active agent instance holds the compacted state.
   - Broadcast the enriched `compaction` event to the event hub.
2. In `SqliteSessionStorage`:
   - `replaceMessages` atomically clears and rewrites the session's `messages` table with the compacted set in a transaction, updating `message_count` and `updated_at`.
3. Reconnection / Reload Resilience:
   - If the user disconnects or server restarts, `loadSession(sessionId)` loads the clean, compacted history with the checkpoint message at the root.

---

## 4. Configuration & Defaults

Extend `CompactionOptions` in `apps/server/agent/src/compaction/index.ts`:

```ts
export interface CompactionOptions {
  /** Enable automatic context window compaction. Default: true */
  enabled?: boolean;
  /** Trigger compaction when estimated tokens reach this ratio of contextWindow. Default: 0.8 (80%) */
  maxThresholdRatio?: number;
  /** Keep recent tokens uncompacted. Default: 20_000 tokens (or 20% of contextWindow) */
  keepRecentTokens?: number;
  /** Hard token threshold override. */
  tokenThreshold?: number;
  /** Max character budget per tool result content. Default: 8000. 0 disables. */
  maxToolResultChars?: number;
  /** Strategy for generating summary text. Default: "structural" */
  summaryStrategy?: "structural" | "llm";
}
```

In `apps/server/agent/src/service/agent.ts`:
```ts
const defaultCompaction: CompactionOptions = {
  enabled: true,
  maxThresholdRatio: 0.8,
  keepRecentTokens: 20_000,
  maxToolResultChars: 8_000,
  summaryStrategy: "structural",
};
```

---

## 5. Wire Event Schema Additions

In `packages/types/src/events.ts`, extend the `compaction` event:

```ts
export interface CompactionEvent {
  type: "compaction";
  summary: string;
  strategy: "structural" | "llm";
  originalMessageCount: number;
  compactedMessageCount: number;
  tokensBefore: number;
  tokensAfter: number;
  compactedMessages: AgentMessage[];
}
```

This is completely additive; existing web/mobile/desktop listeners remain backward-compatible while gaining the ability to render a clean `── Compacted ──` divider.

---

## 6. Implementation Plan & Work Phases

### Phase 1: In-Place Tool Result Truncation & Token Estimation
- Implement `apps/server/agent/src/utils/text-truncate.ts` (`truncateHeadTail(text, maxChars)`).
- Apply truncation to incoming tool results in `apps/server/agent/src/service/agent-loop.ts`.
- Update `estimateMessageTokens` to accurately account for tool result truncations and image attachments.
- **Verification**: Tests in `apps/server/tests/compaction-truncation.test.ts`.

### Phase 2: Turn-Safe Cut-Point Engine
- Implement `apps/server/agent/src/compaction/cut-point.ts` (`findCutPoint`).
- Guarantee never splitting at `toolResult` or orphaning tool call pairs.
- Implement turn boundary validation and fallback to user turn start.
- **Verification**: Tests in `apps/server/tests/compaction-cutpoint.test.ts`.

### Phase 3: File Operation Tracking & Structural Summary
- Implement `apps/server/agent/src/compaction/file-tracker.ts` (`extractFileOps`, `formatFileTree`).
- Implement `apps/server/agent/src/compaction/structural-summary.ts`.
- Construct the prompt-cache friendly wrapper template.
- Implement `compactHistory` using `findCutPoint` and structural summary.
- **Verification**: Tests in `apps/server/tests/compaction-summary.test.ts`.

### Phase 4: State Synchronization & Persistence
- Update `AgentLoopConfig` to accept `compaction: CompactionOptions`.
- Wire `agent-loop.ts` compaction execution to emit `compactedMessages`.
- Update `apps/server/agent/src/service/agent.ts` to sync `this._messages` on compaction.
- Update `apps/server/api/src/services/run.service.ts` to invoke `sessionStorage.replaceMessages` on compaction.
- Enable compaction by default in `AgentOptions`.
- **Verification**: Integration test in `apps/server/tests/compaction-lifecycle.test.ts`.

### Phase 5: UI & Client Visibility
- Add divider handling for `compaction` event in mobile `chat-message-list.tsx` and desktop `message_list.rs`.
- Render a clean, collapsible checkpoint indicator showing token savings.
- Run mobile export verification (`bunx expo export --platform android`).

---

## 7. Verification Checklist

1. **Turn Integrity**:
   - `findCutPoint` tested on synthetic histories with:
     - Mid-tool execution sequences (`user -> assistant [call] -> toolResult`).
     - Multiple consecutive tool calls in a single assistant turn.
     - Long histories exceeding 100 turns.
2. **Provider API Compatibility**:
   - Verify converted messages pass provider strict role alternation validation for:
     - Anthropic (no consecutive user/user or assistant/assistant messages, no orphan tool results).
     - Gemini (role alternating `user` / `model`).
     - OpenAI CoreMessage / Responses format.
3. **Persistence Round-Trip**:
   - Run a test session triggering compaction.
   - Reload session from `SqliteSessionStorage` and assert message count matches compacted count.
   - Run turn N+1 and verify agent completes without context re-inflation.
4. **Mobile & Desktop Compilation**:
   - `cd apps/server && bun test tests/compaction*.test.ts`
   - `cd apps/mobile && bunx expo export --platform android`
