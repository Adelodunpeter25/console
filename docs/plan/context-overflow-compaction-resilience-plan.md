# Context Overflow & Compaction Resilience Implementation Plan

## 1. Problem & Root Cause Analysis

In long agent sessions or turns with high-volume tool outputs (e.g. reading multiple files, running verbose test suites, or large directory listings), calls to the Antigravity provider fail with:
```
400 Bad Request: input token count exceeds 1048576 (or context_length_exceeded)
```

The harness already has auto-compaction (`compactHistory`), but fails to prevent or recover from this error due to 4 systemic gaps:

1. **Wire Payload Undercounting in Estimator**:
   `estimateMessageTokens` in `token-estimator.ts` only counts `messages` using a naive `~4 chars/token` estimate. It misses:
   - System prompt & instructions (`ANTIGRAVITY_SYSTEM_INSTRUCTION`, `AGENTS.md`, workspace tree, rule inventories), which alone take 15k–45k tokens.
   - Tool definitions & JSON schemas (15+ tools), which take 5k–15k tokens.
   - Code/stack-trace token density: source code and tool outputs tokenize at ~2.8–3.2 chars/token rather than 4 chars/token.
   - The loop estimates ~850k tokens when the wire payload actually exceeds 1,048,576 tokens.

2. **Compaction Only Runs Pre-Turn (No Mid-Turn Guard & No Error Recovery)**:
   Compaction checks once at the beginning of a user turn. If a tool call during the turn produces a large result (e.g. 150KB from `readFile` or `grep`), the subsequent model call immediately overflows. When the provider returns HTTP 400, the harness crashes the session instead of compacting and retrying.

3. **Short-Session Bypass (`messages.length <= 4`)**:
   `compactHistory()` exits early when `messages.length <= 4` or `firstKeptIndex === 0`. If a user prompt in a fresh session triggers 2 large tool reads, `messages.length` is 3, completely bypassing compaction and overflowing the context limit.

4. **Shake Spares Recent Turns by Default**:
   `shakeConversation()` protects the last 3 user turns (`protectedRecentStart`), so oversized tool outputs generated in the current or preceding turn are left completely intact.

---

## 2. Architecture & Design

```
                     ┌──────────────────────────────────────────────┐
                     │          Agent Turn Cycle (agent-loop)       │
                     └──────────────────────┬───────────────────────┘
                                            │
                                            ▼
                     ┌──────────────────────────────────────────────┐
                     │  Full-Payload Token Estimation & Safety Gate │
                     │  (Messages + System Prompt + Tools + Buffer) │
                     └──────────────────────┬───────────────────────┘
                                            │
               ┌────────────────────────────┴────────────────────────────┐
               │                                                         │
   [Estimated Tokens < Safe Budget]                          [Estimated Tokens >= Safe Budget]
               │                                                         │
               ▼                                                         ▼
    ┌──────────────────────┐                                  ┌──────────────────────┐
    │   streamOneTurn()    │                                  │ Multi-Tier Compaction│
    └──────────┬───────────┘                                  │ 1. Mechanical Shake  │
               │                                              │ 2. History Summary   │
     [HTTP 400 / 413 / Cap]                                   └──────────┬───────────┘
               │                                                         │
               ▼                                                         ▼
    ┌──────────────────────┐                                  ┌──────────────────────┐
    │  Emergency Recovery  │                                  │   streamOneTurn()    │
    │  - Emergency Shake   ├─────────────────────────────────►│      (Retry 1x)      │
    │  - Forced Truncation │                                  └──────────────────────┘
    └──────────────────────┘
```

### 2.1. Full-Payload Token Estimator (`estimatePayloadTokens`)
- Replaces naive message-only estimation with comprehensive payload accounting:
  - Text parts: ~3.2 chars/token for code, markdown, and tool outputs.
  - System prompt & instructions: accurately measured.
  - Tool schemas: serialized JSON schema token cost.
  - Image parts: dynamic estimation (1,600 tokens per standard image, 2,400 for high-res).
- Model-specific safety ceilings:
  - 1,048,576 Gemini models: safe trigger at **800,000 tokens** (~76% capacity, leaving ~248k headroom for thinking + response).
  - 250,000 Claude models: safe trigger at **190,000 tokens** (~76% capacity).
  - 200,000 OpenCode models: safe trigger at **160,000 tokens**.

### 2.2. Mid-Turn Pre-Request Budget Check
- In `agent-loop.ts`, evaluate `shouldCompactPayload` before *every* model request:
  - Turn start (prior to first assistant stream).
  - Mid-turn (after tool executions and before streaming the subsequent response).
- If bloated tool results pushed the context over threshold, run mechanical shake immediately before dispatching the request.

### 2.3. Universal Tool Output Ceiling & Short-Session Shaking
- Decouple **Mechanical Shaking** from **History Summarization**:
  - History summarization requires multiple turns (`messages.length > 4`).
  - Tool output shaking / head-tail truncation applies **unconditionally**, even on turn 1 or single-message sessions.
- Enforce hard per-tool result limit (default 32,000 chars / ~10k tokens) and per-turn total tool ceiling.

### 2.4. Automatic Context Overflow Catch-and-Retry (Emergency Recovery)
- In `agent-loop.ts` / `stream-turn.ts`, intercept provider context errors:
  - Patterns: `input token count exceeds`, `context_length_exceeded`, `prompt_too_long`, `maximum context length`, `HTTP 400 INVALID_ARGUMENT` with token errors, `HTTP 413`.
- When triggered:
  1. Log warning and emit a `compaction` telemetry event.
  2. Execute **Emergency Shake**: truncate all tool outputs across the entire history (including the recent turn) down to 2,000 characters.
  3. Execute `compactHistory` with aggressive token target.
  4. Retry `streamOneTurn()` once.
  5. If retry succeeds, resume execution transparently without surfacing a fatal error to the user.

---

## 3. Implementation Steps

### Step 1: Accurate Full-Payload Estimator (`apps/server/agent/src/compaction/token-estimator.ts`)
1. Implement `estimatePayloadTokens({ messages, systemPrompt, tools, model })`:
   - Compute characters in system prompt, tool definitions, and message parts.
   - Use `CHARS_PER_TOKEN = 3.2` for tool results/code and `3.8` for plain chat.
   - Calculate image attachment costs based on dimensions / base64 payload size.
2. Update `shouldCompact` in `index.ts` to accept full payload context (`systemPrompt`, `tools`).

### Step 2: Unconditional Mechanical Shake (`apps/server/agent/src/compaction/shake.ts`)
1. Update `shakeConversation` to accept an `emergency: boolean` flag:
   - When `emergency: true`: ignore `protectedRecentStart` and truncate oversized tool outputs in the recent turn.
2. Update `compactHistory` in `index.ts`:
   - Always run `shakeConversation` first.
   - If `messages.length <= 4`, return the shaken messages rather than aborting completely.

### Step 3: Mid-Turn Budget Check & Emergency Retry (`apps/server/agent/src/service/agent-loop.ts`)
1. Extract turn execution and budget checking into a helper function `ensureContextBudget(...)`.
2. Run `ensureContextBudget` before the initial turn and after each `toolExecutionEnd`.
3. Wrap `streamOneTurn` in a context-overflow recovery handler:
   ```ts
   try {
     assistantMessage = await streamOneTurn(...);
   } catch (err) {
     if (isContextOverflowError(err) && !attemptedRetry) {
       attemptedRetry = true;
       // 1. Run emergency aggressive shake
       messages = emergencyShake(messages);
       // 2. Compact history
       compactHistory(messages, { keepRecentTokens: 20_000 });
       // 3. Retry turn
       assistantMessage = await streamOneTurn(...);
     } else {
       throw err;
     }
   }
   ```
4. Define `isContextOverflowError(err)` to identify 400/413/context limit messages across Google CCA, Anthropic, OpenAI, and OpenCode.

### Step 4: Model Registry Safety Buffers (`apps/server/agent/src/commands/provider-registry.ts`)
1. Define explicit `safetyTokenThreshold` on models in `provider-registry.ts`:
   - `gemini-*`: 800,000 tokens (for 1,048,576 limit).
   - `claude-*`: 190,000 tokens (for 250,000 limit).
   - `opencode` models: 160,000 tokens (for 200,000 limit).
2. Wire these thresholds into `AgentLoopConfig` / `CompactionOptions`.

---

## 4. Verification & Testing

1. **Token Estimation Unit Tests** (`apps/server/tests/compaction-estimator.test.ts`):
   - Verify payload estimation includes system prompt, tool definitions, and dense code.
   - Verify safety threshold triggers at ~800k for 1M models.

2. **Short-Session Truncation Test** (`apps/server/tests/compaction-short-session.test.ts`):
   - Feed a 2-message history with 1.5MB tool result.
   - Verify tool result is shaken/truncated and does not throw `History too short`.

3. **Catch-and-Retry E2E Test** (`apps/server/tests/compaction-lifecycle.test.ts`):
   - Mock streamFn that throws `400 input token count exceeds 1048576` on first attempt and succeeds on second attempt after compaction.
   - Verify agent loop recovers and completes the run seamlessly.

4. **Integration Run**:
   - Execute `bun apps/server/tests/compaction-lifecycle.test.ts` and `bun apps/server/tests/agent-loop.test.ts`.
