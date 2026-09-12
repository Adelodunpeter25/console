# Advanced Context Compaction & Resilience Architecture Plan

## 1. Executive Summary & Root Cause Analysis

In long multi-turn agent sessions or turns with high-volume tool outputs (e.g. reading multiple files, running verbose test suites, or large directory listings), calls to the Antigravity provider (and other LLM backends) fail with:
```
400 Bad Request: input token count exceeds 1048576 (or context_length_exceeded / prompt_too_long)
```

The harness has auto-compaction (`compactHistory`), but fails to prevent or recover from this error due to 5 systemic gaps:

1. **Wire Payload Undercounting in Estimator**:
   `estimateMessageTokens` in `token-estimator.ts` only counts `messages` using a naive `~4 chars/token` heuristic. It misses:
   - System prompt & instructions (`ANTIGRAVITY_SYSTEM_INSTRUCTION`, `AGENTS.md`, workspace tree, rule inventories), which alone consume 15k–45k tokens.
   - Tool definitions & JSON schemas (15+ tools), which consume 5k–15k tokens.
   - Code & stack-trace token density: source code and tool outputs tokenize at ~2.8–3.2 chars/token rather than 4 chars/token.
   - The loop estimates ~850k tokens while the actual wire payload exceeds 1,048,576 tokens.

2. **Compaction Only Runs Pre-Turn (No Mid-Turn Guard & No Error Recovery)**:
   Compaction checks once at the start of a user turn. If tool calls during the turn produce a large result (e.g. 150KB from `readFile` or `grep`), the subsequent model call immediately overflows. When the provider returns HTTP 400, the harness crashes the session instead of recovering and retrying.

3. **Short-Session Bypass (`messages.length <= 4`)**:
   `compactHistory()` exits early when `messages.length <= 4` or `firstKeptIndex === 0`. If a user prompt in a fresh session triggers large tool reads, `messages.length` is 3, completely bypassing compaction and overflowing the context limit.

4. **Shake Spares Recent Turns by Default**:
   `shakeConversation()` protects the last 3 user turns (`protectedRecentStart`), so oversized tool outputs generated in the current or preceding turn are left completely unpruned.

5. **Incomplete Output Truncation (`stopReason: "length"`)**:
   When the context or generation limit cuts off an assistant response midway through JSON tool arguments, the session gets stranded with a broken syntax error on the subsequent turn.

---

## 2. Core Architectural Pillars

```
                     ┌─────────────────────────────────────────┐
                     │            Session Lifecycle            │
                     │  (Pre-Turn, Mid-Turn, Post-Turn, Error) │
                     └────────────────────┬────────────────────┘
                                          │
                   Threshold / Overflow / Incomplete Trigger
                                          │
                                          ▼
                     ┌─────────────────────────────────────────┐
                     │          Compaction Pipeline            │
                     │             (Method Order)              │
                     └────────────────────┬────────────────────┘
                                          │
          ┌───────────────────────────────┼───────────────────────────────┐
          │                               │                               │
          ▼                               ▼                               ▼
    Tier 1: Shake                 Tier 2: Snapcompact             Tier 3: Summarize / Handoff
    (Mechanical Elision)      (Visual Archival — experiment)       (LLM / Structural Checkpoint)
    - Zero-latency           - Dense bitmap PNG frames            - Fast auxiliary model
    - Replaces bloated tools - 8x13 pixel font                    - Cumulative file tracker
    - Evicts dead outputs    - Vision-token efficient             - Strict role alternation
                            - Measurement-gated: only if
                              shake+summarize prove insufficient
```

### 2.1. Trigger Points
Compaction must run at multiple lifecycle boundaries rather than only at turn start:

| Trigger | Condition | Action |
|---|---|---|
| **Pre-Turn Threshold** | Estimated tokens > `maxThresholdRatio` (e.g. 76%–80%) | Run standard pipeline (Shake → Summarize) |
| **Mid-Turn Budget Check** | Post-tool-execution token sum exceeds safety ceiling | Immediate mechanical shake of bloated outputs before next stream |
| **Overflow Recovery (Catch & Retry)** | Provider returns 400/413 / `context_length_exceeded` | Emergency shake (all turns) + aggressive compaction, retry turn 1x |
| **Incomplete Turn** | Model returns `stopReason === "length"` | Trim partial output, compact context, request continuation |
| **Manual Trigger** | User executes `/compact` command | Force full structural/LLM summarization pass |

### 2.2. Multi-Stage Pipeline (`methodOrder`)
Compaction executes through a prioritized sequence rather than jumping straight to expensive summarization:

```ts
export type CompactionMethod = "shake" | "snapcompact" | "summarize" | "handoff";

export interface CompactionConfig {
  enabled: boolean;
  maxThresholdRatio: number; // e.g. 0.76 (76% for 1M models -> 800k tokens)
  keepRecentTokens: number;  // default: 40_000
  methodOrder: CompactionMethod[]; // ["shake", "summarize"]
  maxToolResultChars: number; // default: 8_000
  emergencyToolResultChars: number; // default: 2_000
}
```

---

## 3. Detailed Component Specifications

### 3.1. Full-Payload Token Estimator (`estimatePayloadTokens`)
- Replaces naive message-only estimation with comprehensive payload accounting:
  - Text parts: counted with a model/provider-aware tokenizer when available.
  - System prompt & instructions: included in the count.
  - Tool schemas: serialized in the same logical shape used for the provider request and counted.
  - Image parts: counted through the provider's multimodal counting path when available; otherwise use a conservative documented estimate.
  - Provider-specific wire overhead: represented by a calibrated safety margin rather than pretending local counts are exact.
- Add a tokenizer policy layer, following the model-metadata pattern used by `oh-my-pi`:
  - Resolve a bounded, cached `tokenizerFamily` from the canonical model identity; do not infer tokenizer behavior repeatedly inside the compaction loop.
  - `openai`: use `gpt-tokenizer` for local BPE counting. It is dependency-free, TypeScript-native, and supports current OpenAI model families, but it must not be used as a claimed exact count for non-OpenAI providers.
  - `gemini`: prefer Google's official `models.countTokens` endpoint through `@google/genai` when credentials and the provider route are available. This counts system instructions, tools, multimodal content, and conversation history in Gemini's own accounting. Cache or debounce counts so this does not become a request before every model call.
  - `anthropic`: prefer the official Messages API token-counting endpoint (`messages.countTokens`) when available. Treat the old `@anthropic-ai/tokenizer` package as legacy/beta fallback only; it is not a substitute for the current provider count and is not suitable as the universal tokenizer.
  - `unknown` / OpenCode-compatible providers: use the conservative fallback estimator and a provider-specific margin until an exact counting endpoint or verified tokenizer is available.
- Counting modes:
  - **Fast local estimate** for ordinary checks and after small tool results.
  - **Provider count** when the local estimate is near the safety threshold, after substantial tool output, or before an emergency-sensitive dispatch. Provider counting must fail open to the conservative local estimate, never block the agent indefinitely.
  - Record whether a decision used `local`, `provider`, or `fallback` counting for telemetry and calibration.
- Model-specific safety ceilings:
  - 1,048,576 Gemini models: safe trigger at **800,000 tokens** (~76% capacity, leaving ~248k headroom for thinking + response).
  - 250,000 Claude models: safe trigger at **190,000 tokens** (~76% capacity).
  - 200,000 OpenCode models: safe trigger at **160,000 tokens**.

### 3.2. Provider Usage Anchoring & Mid-Turn Pre-Request Budget Check
- Use the latest valid provider-reported assistant usage as an authoritative anchor for the already-counted context.
- Estimate only messages appended after that usage record with the model-aware estimator; do not repeatedly re-estimate the complete historical transcript when a valid usage anchor exists.
- Fall back to full-payload estimation when no valid usage anchor exists or when the wire payload has materially changed outside the measured history.
- In `agent-loop.ts`, evaluate `shouldCompactPayload` before *every* model request:
  - Turn start (prior to first assistant stream).
  - Mid-turn (after tool executions and before streaming the subsequent response).
- If bloated tool results pushed the context over threshold, run mechanical shake immediately before dispatching the request.
- Record the estimate source and usage-anchor index in compaction telemetry.

### 3.3. Boundary-Safe Cut Points & Durable Compaction Preparation
- Select compaction cuts only at valid message/turn boundaries; never orphan a `toolResult` from its preceding assistant `toolCall`.
- Detect when the cut splits an in-progress turn and persist the turn-prefix messages separately from the retained tail.
- Persist the complete compaction preparation before starting the summarization request:
  - Messages to summarize.
  - Split-turn prefix messages.
  - Retained tail.
  - Previous summary.
  - Token estimate and count source.
  - File-operation facts.
  - Settings and recovery attempt metadata.
- Treat compaction as a resumable workflow, not an in-memory transformation. A restart must resume the prepared task or safely abandon it without losing the original transcript.
- Reference implementation: `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/compaction/compaction.ts`, especially `estimateContextTokens`, `findCutPoint`, and `prepareCompaction`; durable workflow orchestration is in `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/runtime/drive/structural.ts`.

### 3.4. Universal Tool Output Ceiling & Short-Session Shaking
- Decouple **Mechanical Shaking** from **History Summarization**:
  - History summarization requires multiple turns (`messages.length > 4`).
  - Tool output shaking / head-tail truncation applies **unconditionally**, even on turn 1 or single-message sessions.
- Enforce hard per-tool result limit (default 32,000 chars / ~10k tokens) and per-turn total tool ceiling.

### 3.5. Durable Context Overflow Recovery
- In `agent-loop.ts` / `stream-turn.ts`, intercept provider context errors:
  - Patterns: `input token count exceeds`, `context_length_exceeded`, `prompt_too_long`, `maximum context length`, `HTTP 400 INVALID_ARGUMENT` with token errors, `HTTP 413`.
- Model the recovery as a durable state transition rather than an in-memory catch-and-retry:
  1. Log the overflow and emit a `compaction` telemetry event.
  2. Persist an overflow recovery task containing the trigger entry/generation, original continuation point, recovery attempt count, and `overflowRecoveryUsed` marker.
  3. Execute **Emergency Shake**: truncate all tool outputs across the entire history, including the recent turn, down to 2,000 characters while retaining truncation metadata.
  4. Prepare and persist boundary-safe compaction inputs before invoking summarization.
  5. Run `compactHistory` with aggressive token target (`keepRecentTokens: 20_000`).
  6. Resume the assistant generation from the persisted continuation point.
  7. Permit at most one overflow recovery per generation/turn; if the resumed request overflows again, finish with a structured provider error instead of looping.
- Reference implementation: `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/runtime/drive/response.ts` (`publishResponse` overflow handling), `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/runtime/drive/structural.ts` (`prepareOverflowCompaction`), and `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/session/types.ts` (durable continuation state).

### 3.6. Interrupted & Incomplete Assistant Output Recovery
- Persist committed assistant frames during streaming so a process interruption does not require reconstructing the response from an invalid final JSON/tool-call payload.
- On interruption or cancellation, read the bounded committed frame prefix, reduce it into the latest partial assistant message, mark it explicitly as interrupted/error, and publish recovery-aware lifecycle events without making an unsafe duplicate provider request.
- Preserve provider identity, model, usage defaults, stop reason, and an actionable warning that the external outcome may be unknown.
- Reference implementation: `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/runtime/drive/recovery.ts` and the frame lifecycle in `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/runtime/drive/response.ts`.

### 3.7. Structured Checkpoint Invariants & Cumulative File Tracker
- Checkpoints preserve critical working state:
  - **Cumulative File Tracker**: Files read, modified, or created are preserved verbatim with their latest state in the structural summary.
  - **Strict Role Alternation**: Output always begins with `role: "user"` (the checkpoint summary) followed by `role: "assistant"` ack, preserving multi-turn invariants for Claude and Gemini.
  - **Turn-Atomicity**: Slicing never orphans a `toolResult` from its preceding `assistant` `toolCall` (`isToolCallSafe` validation).

### 3.8. Prompt-Cache Awareness
- **Prefer user-turn boundaries**: Post-turn maintenance should only rewrite context at user/assistant turn boundaries, never inside a streaming turn unless overflow is imminent.
- **Hysteresis**: After a compaction, require usage to re-cross a second, higher threshold (e.g. 0.85) before another pass runs, preventing oscillation between compact and full context.
- **Emergency Exemption**: Mid-turn emergency shake (to avoid a 400) is exempt from the boundary preference: avoiding a fatal provider failure takes priority over cache preservation.

### 3.9. Observability & Telemetry
- Emit a `CompactionEntry` in SQLite session storage **and** a `compaction` session event on the run SSE stream whenever any tier fires:
  ```json
  {
    "type": "compaction",
    "tier": "shake" | "summarize" | "emergency_recovery",
    "trigger": "pre_turn" | "mid_turn" | "overflow_retry" | "length_continuation",
    "tokensBefore": 880000,
    "tokensAfter": 420000,
    "tokenCountSource": "local" | "provider" | "fallback",
    "usageAnchorIndex": 18,
    "compactedMessageCount": 12,
    "originalMessageCount": 45,
    "recoveryAttempt": 0,
    "overflowRecoveryUsed": false,
    "summary": "..."
  }
  ```
- Server-side logging correlates compaction passes with provider latency and token consumption.
- Include `tokenCountSource`, `usageAnchorIndex`, `recoveryAttempt`, and `overflowRecoveryUsed` in telemetry so estimator and recovery decisions are diagnosable without logging prompt contents.

---

## 4. Implementation Steps

### Step 1: Accurate Full-Payload Estimator (`apps/server/agent/src/compaction/token-estimator.ts`)
1. Define model-aware metadata and interfaces:
   - `TokenizerFamily = "openai" | "gemini" | "anthropic" | "fallback"`.
   - `TokenCountSource = "local" | "provider" | "fallback"`.
   - `estimatePayloadTokens({ messages, systemPrompt, tools, model, counter })` returns both the count and source metadata.
2. Implement a bounded cached tokenizer-family resolver based on canonical model/provider identity, following `oh-my-pi`'s `resolveModelTokenizer` pattern.
3. Implement local counting with `gpt-tokenizer` only for OpenAI-compatible models. Do not use it for Gemini or Anthropic merely because it is available.
4. Add provider counter adapters:
   - Gemini: `models.countTokens` via `@google/genai`, with the exact contents/system-instruction/tools representation sent to Gemini.
   - Anthropic: `messages.countTokens` through the official SDK/API, including system and tools.
   - Return a typed unavailable/error result rather than throwing from the estimator.
5. Keep the conservative fallback for unsupported providers and provider-count failures:
   - Use density-specific estimates for plain text, code/JSON/tool output, and multimodal parts.
   - Apply a configurable wire-overhead/safety margin.
6. Add a count policy that uses local counting normally and provider counting near thresholds or after large tool-output changes. Debounce/cache equivalent payload counts.
7. Update `shouldCompact` in `index.ts` to accept full payload context (`systemPrompt`, `tools`) and preserve count-source metadata in compaction telemetry.

### Step 2: Boundary-Safe Compaction Preparation (`apps/server/agent/src/compaction/index.ts`)
1. Add provider-usage anchoring to `estimateContextTokens`:
   - Use the latest valid assistant usage block as the measured prefix.
   - Token-count only the unmeasured tail with the model-aware estimator.
   - Return estimate source, usage-anchor index, and confidence metadata.
2. Add valid cut-point discovery:
   - Never cut between an assistant tool call and its tool result.
   - Detect split turns and retain the turn prefix separately.
   - Preserve summary/retained-tail ordering and role invariants.
3. Make `prepareCompaction` produce a durable preparation record before summarization.
4. Reference implementation: `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/compaction/compaction.ts`.

### Step 3: Unconditional Mechanical Shake and Rich Truncation (`apps/server/agent/src/compaction/shake.ts`)
1. Update `shakeConversation` to accept an `emergency: boolean` flag:
   - When `emergency: true`: ignore `protectedRecentStart` and truncate oversized tool outputs in the recent turn down to 2,000 characters.
2. Preserve structured truncation metadata:
   - `truncated`, reason/limit, original and output line counts, original and output byte counts, range/fingerprint, and continuation information.
   - Use UTF-8 byte counts and complete-line boundaries where possible.
3. Update `compactHistory` in `index.ts`:
   - Always run `shakeConversation` first.
   - If `messages.length <= 4`, return the shaken messages rather than aborting completely.
4. Reference implementation: `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/utils/truncate.ts`.

### Step 4: Durable Overflow Recovery (`apps/server/agent/src/service/agent-loop.ts` and runtime state)
1. Extract turn execution and budget checking into a helper function `ensureContextBudget(...)`.
2. Run `ensureContextBudget` before the initial turn and after each `toolExecutionEnd`.
3. Detect context overflow in both rejected requests and settled provider responses.
4. Persist an overflow recovery task before mutating context:
   - Trigger generation/entry id.
   - Continuation point.
   - Recovery attempt count.
   - `overflowRecoveryUsed` guard.
5. Execute emergency shake, prepare durable compaction, summarize, then resume from the persisted continuation point.
6. Allow exactly one overflow recovery per generation. A second overflow becomes a structured terminal provider error.
7. Define `isContextOverflowError(err)` to identify 400/413/context limit messages across Google CCA, Anthropic, OpenAI, and OpenCode.
8. Reference implementation: `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/runtime/drive/response.ts`, `structural.ts`, and `session/types.ts`.

### Step 5: Interrupted Assistant Generation Recovery (`apps/server/agent/src/service/agent-loop.ts` and runtime state)
1. Persist bounded committed assistant frames during streaming.
2. On interruption/cancellation, reconstruct the latest partial assistant message from committed frames.
3. Mark it as an explicit interrupted/error response and emit recovery-aware lifecycle events without issuing an unsafe duplicate request.
4. Reference implementation: `/Users/adelodunpeter/Developer/Projects/pi/packages/agent/src/harness/runtime/drive/recovery.ts` and `response.ts`.

### Step 6: Model Registry Safety Buffers (`apps/server/agent/src/commands/provider-registry.ts`)
1. Define explicit `safetyTokenThreshold` on models in `provider-registry.ts`:
   - `gemini-*`: 800,000 tokens (for 1,048,576 limit).
   - `claude-*`: 190,000 tokens (for 250,000 limit).
   - `opencode` models: 160,000 tokens (for 200,000 limit).
2. Wire these thresholds into `AgentLoopConfig` / `CompactionOptions`.

---

## 5. Verification & Test Plan

1. **Token Estimation Unit Tests** (`apps/server/tests/compaction-estimator.test.ts`):
   - Verify payload estimation includes system prompt, tool definitions, and dense code.
   - Verify the tokenizer-family resolver selects OpenAI, Gemini, Anthropic, and fallback policies correctly.
   - Verify OpenAI local counting uses `gpt-tokenizer` and does not silently apply to Gemini or Anthropic.
   - Verify provider count adapters include system instructions, tools, multimodal parts, and full history.
   - Verify provider-count failure falls back to a conservative local estimate with source metadata.
   - Verify equivalent payloads use the bounded count cache and do not issue duplicate provider count requests.
   - Verify safety threshold triggers at ~800k for 1M models.

2. **Short-Session Truncation Test** (`apps/server/tests/compaction-short-session.test.ts`):
   - Feed a 2-message history with 1.5MB tool result.
   - Verify tool result is shaken/truncated and does not throw `History too short`.

3. **Durable Compaction and Recovery Tests** (`apps/server/tests/compaction-lifecycle.test.ts`):
   - Mock a compaction preparation, restart/resume the workflow, and verify the persisted preparation is reused without rereading or recomputing the original history.
   - Verify cut-point selection never separates an assistant tool call from its tool result and preserves split-turn prefixes.
   - Mock a provider response that reports `400 input token count exceeds 1048576` and verify durable emergency compaction resumes exactly once.
   - Verify a second overflow produces a structured terminal error and cannot loop.
   - Verify committed partial assistant frames recover into an explicit interrupted/error message without a duplicate provider request.

4. **Truncation and Recovery Manifest Tests** (`apps/server/tests/compaction-truncation.test.ts`):
   - Verify UTF-8 byte and line metadata, stable fingerprints, continuation ranges, and duplicate-read protection.
   - Verify a truncated read requests the next non-overlapping range rather than repeating the original full-file request.

5. **Integration Run**:
   - Execute `bun apps/server/tests/compaction-lifecycle.test.ts` and `bun apps/server/tests/agent-loop.test.ts`.

---

## 6. Harness Compaction Incident: Repeated Reads After Context Loss

The following incident is part of the resilience scope and must be treated as a harness failure mode, not as normal agent behavior:

> Yes — part of it is the harness, not just me.
>
> What happened: after the conversation got compacted, I kept the list of files we touched but lost their actual contents. So I tried to re-read the same 4 files to rebuild context.
>
> Each read came back truncated, so I assumed it failed and retried the same read again — that's the loop you saw.
>
> I'm stopping that now. I have enough from the SSE plan doc and the registry code I did get to move forward, I won't re-read anything unless it's new.

### Root cause

Compaction preserved references to touched files but did not preserve enough usable content, summaries, or read results. The agent then treated a truncated tool response as a transient read failure. Because there was no retry budget, duplicate-request detection, or truncation-aware state, it repeated the same reads indefinitely.

This is distinct from provider context overflow:

- **Provider overflow** means the outgoing model payload is too large.
- **Harness read looping** means the agent repeatedly requests the same resource after context reconstruction failed.

### Tokenizer/package research notes

- `gpt-tokenizer` is the strongest local JavaScript candidate for OpenAI-family models: it is TypeScript-native, dependency-free, and exposes `countTokens`/encoding APIs. It should be scoped to OpenAI-compatible model families rather than treated as a universal tokenizer.
- `@anthropic-ai/tokenizer` exists, but the published package is an older beta-era text tokenizer. Prefer Anthropic's current official token-counting API for Claude payloads; retain the package only as an explicitly labeled fallback if compatibility testing justifies it.
- Gemini's official `models.countTokens` API is preferable to a third-party local package because it can count the actual Gemini representation, including system instructions, tools, multimodal content, and multi-turn history. Google documents that the endpoint has no charge/quota restriction and supports up to 3,000 requests per minute, but the implementation should still cache/debounce calls.
- No credible universal local tokenizer was identified that can accurately count all providers' wire payloads. `tokenx` is useful for approximate splitting/estimation, not provider-exact accounting, and should not be selected as the primary estimator.
- Package selection must be validated against Bun compatibility, bundle size/startup cost, licensing, model coverage, and representative golden payloads before adding dependencies.

Both need explicit recovery controls.

### Required fixes

1. **Persist compact file facts, not only file names**
   - During compaction, retain a bounded per-file record containing path, relevant line ranges, definitions/signatures, the latest successful read result, and a short summary.
   - Mark each record as `complete`, `truncated`, or `unread`.
   - Keep this recovery manifest outside the conversational message history so compaction cannot discard it.

2. **Make truncation an explicit tool result**
   - A `readFile`/search result must include metadata such as `truncated`, `startLine`, `endLine`, `totalLines`, and a stable content fingerprint when available.
   - The model should be told that truncation is expected and how to request the next range; a truncated response must not look like a failed request.

3. **Add duplicate-read loop protection**
   - Track a request key made from tool name, normalized path, range, and relevant search parameters.
   - Allow at most one automatic retry for the same key.
   - If the same key is requested again after a truncated response, return a structured diagnostic explaining that the response was already received and recommend a narrower or adjacent range.
   - Cap repeated reads per file and per turn, with a clear stop reason when the cap is reached.

4. **Use continuation reads instead of whole-file retries**
   - When a result is truncated, automatically suggest or schedule the next non-overlapping range.
   - Never retry the original full-file request merely because the response was truncated.
   - Preserve the already received lines and request only the missing range.

5. **Add compaction recovery state**
   - Store `contextRecovery` state with the compaction checkpoint: touched files, preserved facts, pending ranges, completed tool keys, and retry counts.
   - On resumed execution, consult this state before issuing a read.
   - If enough evidence exists to continue safely, proceed without rereading; if not, ask for a targeted range rather than starting a read loop.

6. **Add tests and observability**
   - Test that a truncated read produces a continuation request rather than the same request.
   - Test that two identical reads are deduplicated after compaction.
   - Test that the retry budget terminates a loop with a structured diagnostic.
   - Log a compact recovery event containing the tool key, truncation state, retry count, and decision (`continue`, `narrow`, `skip`, or `stop`). Do not log file contents or secrets.

### Acceptance criteria

- A compacted session can continue from preserved file facts without blindly rereading every touched file.
- A truncated tool response is never interpreted as an unknown or failed response.
- The same read request cannot repeat indefinitely within a turn or recovery cycle.
- Continuation reads are range-based and non-overlapping.
- When recovery cannot proceed, the harness stops with an actionable diagnostic instead of consuming the remaining context window.
