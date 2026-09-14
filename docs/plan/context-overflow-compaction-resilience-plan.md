# Context Overflow & Compaction Resilience Plan

## 0. Status & Scope

**Goal:** long sessions never die with a context-overflow 400, and compaction runs fully automatically. No manual `/compact` command — auto-compact must just work.

**Providers in scope:** Antigravity (Gemini), Claude (Anthropic subscription OAuth), Codex (OpenAI/ChatGPT), OpenCode. Devin and Cline are disabled and out of scope.

**Current state (verified against code, Sep 2026):**

- [x] Naive message-only estimator (`agent/src/compaction/token-estimator.ts` — `chars/4`, no system/tools)
- [x] Pre-turn-only compaction check (`agent/src/service/agent-loop.ts`)
- [x] Mechanical shake, 8k cap, spares recent turns (`agent/src/compaction/shake.ts`)
- [x] Boundary-safe cut points (`cut-point.ts` — `isToolCallSafe`, never orphans toolResult)
- [x] Structural summaries + file tracker (`structural-summary.ts`, `file-tracker.ts`)
- [x] Basic tests (`compaction-cutpoint/lifecycle/summary/truncation.test.ts`)
- [ ] Everything below

**Decisions (locked):**

- **P0 uses no local tokenizer packages.** Fixes ship on the payload-complete heuristic + provider count endpoints. Tokenizer packages land in Phase 1, after the crash fixes (see T0) — sequenced, not dropped.
- **No manual `/compact` command.** All triggers automatic.
- **No snapcompact/visual archival.** Parked as out of scope (see §5).
- **No durable restart-recovery workflow yet.** Overflow recovery is in-memory per turn (retry-once guard); persisted workflow orchestration is deferred to Phase 2.

## 1. Bugs First (P0) — fix before any new features

### B0. Shake bypasses short sessions and recent turns
**Problem:** `shakeConversation` spares the last 3 user turns via `protectedRecentStart`, and `compactHistory` (`index.ts:84`) returns early for `messages.length <= 4`. A 150KB tool result on turn 1 sails through untouched and overflows.
**Fix:**
- `shakeConversation(messages, maxChars, { emergency })`: `emergency: true` ignores the protected suffix and truncates oversized tool outputs everywhere down to `emergencyToolResultChars` (2,000).
- `compactHistory`: always shake first (already does); when `messages.length <= 4`, still return shaken messages **and** report whether shaking alone got under threshold so the caller can escalate.
**Acceptance:** 2-message history with a 1.5MB tool result compacts without throwing and without "History too short" aborting the pipeline.

### B1. No mid-turn budget check
**Problem:** compaction is evaluated once at turn start (`agent-loop.ts:123`). A large tool result mid-turn overflows the very next model call.
**Fix:** after each `toolExecutionEnd`, re-estimate payload tokens; if over the safety ceiling, run mechanical shake (non-emergency) before the next stream. Cheap path: fast local estimate only (see B3).
**Acceptance:** a turn whose tool outputs push context over threshold shakes before the follow-up request instead of 400ing.

### B2. Overflow 400 crashes the session
**Problem:** provider context errors (`input token count exceeds`, `context_length_exceeded`, `prompt_too_long`, `maximum context length`, HTTP 400/413 token errors) propagate as fatal session errors.
**Fix:**
- `isContextOverflowError(err)` matching overflow signatures across Antigravity/CCA, Anthropic, OpenAI/Codex, OpenCode.
- On match: emergency shake (B0) + aggressive `compactHistory` (`keepRecentTokens: 20_000`), resume the turn **once**. Second overflow in the same turn → structured terminal provider error (never loop).
**Acceptance:** mocked `400 input token count exceeds 1048576` triggers emergency compaction and exactly one retry; a second overflow ends the turn with a clear error, not a crash or loop.

### B3. Estimator undercounts the wire payload
**Problem:** `estimateMessageTokens` counts messages only at `chars/4`. Missing: system prompt + instructions (15k–45k tokens), tool schemas (5k–15k), denser code tokenization (~3 chars/token), image parts.
**Fix (no tokenizer deps):**
- `estimatePayloadTokens({ messages, systemPrompt, tools })`: prose ≈ 4 chars/token, code/JSON/tool output ≈ 3 chars/token, plus a per-provider wire-overhead margin. Counts system, tools, and history together.
- Near-threshold escalation only: use provider-native count endpoints, fail open to the heuristic —
  - Anthropic: `POST /v1/messages/count_tokens` (same OAuth bearer + headers; verify against subscription tokens during implementation),
  - Antigravity/Gemini: `models.countTokens` REST where the credential permits; CCA has no count endpoint, so heuristic + margin is the primary path,
  - Codex/OpenCode: heuristic + larger margin (no count endpoint on those APIs).
- Cache/debounce counts; record count source (`local` | `provider`) in telemetry.
**Acceptance:** a payload with 30k system tokens + 10k tool schemas + 800k history estimates ≥ 840k (old code reports ~800k); provider-count failure never blocks a turn.

### B4. Anthropic rejects empty `tool_result` blocks
**Problem:** after shake truncation, a `tool_result` can reach the Claude provider with empty content, which the Messages API rejects with 400. (oh-my-pi carries the same guard: `EMPTY_ERROR_TOOL_RESULT_TEXT`.)
**Fix:** in the Claude converter, replace empty `tool_result` content with `"Tool failed with no output."`; skip empty text blocks (already done for assistant parts — audit user parts too).
**Acceptance:** unit test sends empty/error tool results through `convertClaudeMessages` and asserts no empty content blocks on the wire.

### B5. Read-looping after compaction (harness bug, not provider overflow)
**Problem (observed incident):** after compaction the agent kept file *names* but lost contents, treated truncated re-reads as failures, and re-read the same files in a loop — no retry budget, no truncation awareness.
**Fix:**
- Truncation becomes explicit metadata on tool results (`truncated`, `startLine`/`endLine`/`totalLines`, content fingerprint), never shaped like a failure.
- Duplicate-request protection: key = tool + normalized path + range; at most one automatic retry per key; repeat after truncation returns a diagnostic pointing at the next non-overlapping range.
- Compaction preserves bounded per-file facts (path, ranges, latest read result, `complete`|`truncated`|`unread`) outside the message history so checkpoints stay usable.
**Acceptance:** truncated read → continuation range request (never the same full read); identical repeat → diagnostic + stop; compacted session continues from preserved file facts without rereading everything.

## 2. Phase 1 — exact local counting with tokenizer packages (after P0)

Once the crash fixes are in, replace the density heuristic with provider-family tokenizers, calibrated against real payloads.

### T0. Adopt tokenizer packages per provider family
- **OpenAI-family (Codex, OpenCode):** tiktoken (or dependency-free `gpt-tokenizer`) — BPE-exact for these models. Scope strictly to OpenAI-compatible payloads.
- **Anthropic:** prefer the `messages.countTokens` endpoint as primary (B3); evaluate `@anthropic-ai/tokenizer` as an offline fallback only, labeled as approximate — the published package is beta-era and not a substitute for the provider count.
- **Antigravity/Gemini:** prefer `models.countTokens` where the credential permits; no credible local Gemini tokenizer exists, so the calibrated heuristic + margin remains the offline path.
- **Calibration:** for each provider, record heuristic-vs-actual deltas on golden payloads (system + tools + history + images) and bake the observed margin into the fast-path estimator, so the local count stays conservative without a package where none is credible.
- **Dependency vetting before adding:** Bun compatibility, bundle size/startup cost, license, model coverage. Fail open to the P0 heuristic on any counting error.
**Acceptance:** estimator tests assert per-family golden payloads within the calibrated margin; no provider silently uses another family's tokenizer (the old plan's explicit anti-goal).

## 3. Hardening (P2) — after Phase 1

### H0. Per-model safety thresholds
Replace the flat `contextWindow * 0.85` in `shouldCompact` with explicit ceilings:
- Antigravity Gemini (1,048,576): **800,000**
- Antigravity Claude (250,000): **190,000**
- Claude provider (200,000): **150,000**
- Codex (272,000): **205,000**
- OpenCode (200,000): **150,000**

### H1. Usage-anchored estimation
Use the latest provider-reported usage block as the measured prefix; estimate only the tail appended since. Falls back to full estimation with no anchor. Avoids re-counting full history every mid-turn check.

### H2. Hysteresis + cache awareness
After a compaction pass, require usage to re-cross a higher threshold (0.85→0.90 band) before compacting again. Emergency mid-turn shake stays exempt — avoiding a 400 beats cache preservation.

### H3. Telemetry enrichment
Extend the existing `compaction` session event with `tier` (`shake` | `summarize` | `emergency_recovery`), `trigger` (`pre_turn` | `mid_turn` | `overflow_retry`), `tokensBefore/After`, `tokenCountSource`, `recoveryAttempt`. No prompt contents in logs.

## 4. Deferred (P3)

- Persisted overflow-recovery workflow (resume across restarts; needs storage design first).
- Interrupted-stream recovery (partial assistant frames on abort).
- Server-side Anthropic `context_management` compaction edits (opt-in beta; revisit once P0 is stable).

## 5. Test Plan

Map to existing files; run with `cd apps/server && bun tests/<name>.test.ts`:

- `compaction-estimator.test.ts` (new): payload completeness (system+tools+history), code density, provider-count fail-open, threshold triggers per H0 values. Phase 1 adds per-family golden-payload accuracy tests (T0).
- `compaction-short-session.test.ts` (new): B0 acceptance (2-message + 1.5MB result).
- `compaction-lifecycle.test.ts` (extend): B1 mid-turn shake, B2 overflow→retry-once→terminal-error.
- `compaction-truncation.test.ts` (extend): truncation metadata shape, continuation-range behavior, duplicate-read diagnostic (B5).
- Claude converter tests (`claude.test.ts`, extend): B4 empty-`tool_result` guard.

## 6. Out of Scope (explicitly parked)

- Snapcompact / visual PNG archival of tool outputs.
- Manual `/compact` command.
- Devin/Cline provider support.
- Persisted cross-restart recovery workflow (see P3).

## 7. Appendix: reference implementations (relative paths)

- `apps/server/agent/src/compaction/` — current engine (`index.ts`, `cut-point.ts`, `shake.ts`, `structural-summary.ts`, `file-tracker.ts`, `token-estimator.ts`)
- `apps/server/agent/src/service/agent-loop.ts` — turn loop, compaction hook at line ~123
- Provider wire code: `apps/server/providers/src/{antigravity→auth,claude,codex,opencode}/`
- Upstream compaction patterns live outside this repo (oh-my-pi `packages/agent`, pi-mono `packages/agent/src/harness/`); consult on demand rather than vendoring.
