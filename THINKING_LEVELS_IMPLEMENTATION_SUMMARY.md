# Thinking Levels Implementation Summary

## Overview
Successfully implemented thinking level support for **Codex (OpenAI)** and **Claude (Anthropic)** providers in the Console agent harness. This complements the existing **Antigravity (Gemini)** implementation.

## What Was Implemented

### ✅ Step 1: Model Registry Update
**File:** `apps/server/agent/src/commands/provider-registry.ts`

Added thinking level constants and model metadata:
- **Codex**: 7 levels supported (`none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`)
- **Claude**: 5 levels supported (`low`, `medium`, `high`, `xhigh`, `max` — no `none`/`minimal`)
- **Gemini**: Already implemented (4 levels: `minimal`, `low`, `medium`, `high`)

Each model now includes:
- `supportedThinkingLevels: ThinkingLevel[]`
- `defaultThinkingLevel: ThinkingLevel` (Codex defaults to `"medium"`, Claude to `"high"`)

### ✅ Step 2: Codex Stream Function
**File:** `apps/server/providers/src/codex/stream-fn.ts`

- Added `mapThinkingLevelToCodex()` function for direct 1:1 mapping
- Updated `buildRequestBody()` to accept optional `thinkingLevel` parameter
- Inserts `reasoning: { type: "enabled", effort: "..." }` into request body when level specified
- Updated `codexStreamFn` to accept and pass through `thinkingLevel`

### ✅ Step 3: Claude Stream Function
**File:** `apps/server/providers/src/claude/stream-fn.ts`

- Added `mapThinkingLevelToClaude()` function that filters out unsupported levels
- Updated `buildRequestBody()` to accept optional `thinkingLevel` parameter
- Inserts `output_config: { effort: "..." }` at top level when level specified
- Handles Claude's lack of `"none"`/`"minimal"` by returning `undefined` to omit the parameter
- Updated `claudeStreamFn` to accept and pass through `thinkingLevel`

### ✅ Step 4-5: Agent Loop Threading (Already Complete)
**Files:** `apps/server/agent/src/service/types.ts`, `stream-turn.ts`, `agent-loop.ts`

The agent loop infrastructure already had thinking level support:
- `StreamFn` type already includes optional `thinkingLevel` parameter
- `StreamParams` interface already includes optional `thinkingLevel` field
- `AgentLoopConfig` already accepts optional `thinkingLevel`
- `streamOneTurn()` already passes through `thinkingLevel` to stream function

### ✅ Step 6: Validation
**File:** `apps/server/agent/src/service/validate-thinking.ts` (new)

Created `validateThinkingLevelForModel()` function that:
- Accepts omitted levels (uses model default)
- Rejects unsupported levels with clear error messages
- Provides helpful feedback listing supported levels

**Integration:** Called in `runAgentLoop()` right after config destructuring for early error detection.

### ✅ Step 7: Comprehensive Backend Tests

#### Validation Tests
**File:** `apps/server/tests/agent/thinking-level-validation.test.ts`

- 24 test cases covering:
  - Codex model support for all 7 levels
  - Claude model support for 5 levels (rejection of `none`/`minimal`)
  - Gemini model support for 4 levels
  - Models without thinking level support
  - Clear error messages with model ID and supported levels

#### Provider-Specific Tests
**Files:**
- `apps/server/tests/providers/codex-thinking-levels.test.ts` (10 tests)
- `apps/server/tests/providers/claude-thinking-levels.test.ts` (14 tests)

Cover:
- Level support verification
- Correct mapping to provider API parameters
- Request body construction with/without thinking levels
- Model registry defaults

**All 48 tests pass successfully.**

## API Mapping Reference

| Console Level | Codex `reasoning.effort` | Claude `output_config.effort` | Gemini `thinkingLevel` |
|---|---|---|---|
| `none` | ✅ `"none"` | ❌ Omitted | ❌ Not supported |
| `minimal` | ✅ `"minimal"` | ❌ Omitted | ✅ `"minimal"` |
| `low` | ✅ `"low"` | ✅ `"low"` | ✅ `"low"` |
| `medium` | ✅ `"medium"` | ✅ `"medium"` | ✅ `"medium"` |
| `high` | ✅ `"high"` | ✅ `"high"` | ✅ `"high"` |
| `xhigh` | ✅ `"xhigh"` | ✅ `"xhigh"` | ❌ Not supported |
| `max` | ✅ `"max"` | ✅ `"max"` | ❌ Not supported |

## Configuration Example

Using thinking levels with the agent:

```typescript
import { runAgentLoop } from "@/agent/src/service/agent-loop.js";

const result = await runAgentLoop("Fix the bug in utils.ts", {
  model: selectedClaudeModel,
  systemPrompt: "You are a helpful coding assistant.",
  tools: [readFile, writeFile, bash],
  streamFn: claudeStreamFn,
  thinkingLevel: "high",  // ← Optional: request high reasoning effort
  approvalMode: "accept-edits",
  onEvent: (event) => console.log(event),
});
```

If `thinkingLevel` is omitted, each model uses its configured default:
- Codex: `"medium"`
- Claude: `"high"`
- Gemini: `"medium"`

## Error Handling

**Invalid thinking level example:**

```typescript
// This will throw during agent loop initialization:
{
  model: claudeModel,
  thinkingLevel: "none",  // ❌ Claude doesn't support "none"
  // ...
}

// Error message:
// Model "claude-opus-4-6" does not support thinking level "none". 
// Supported levels: low, medium, high, xhigh, max
```

## Backward Compatibility

✅ **Fully backward compatible:**
- All existing code without `thinkingLevel` specified continues to work
- Models without thinking level support (e.g., older models) work as before
- Type definitions mark `thinkingLevel` as optional
- Omitted thinking levels automatically use model's configured default

## Next Steps (Deferred to Future Phases)

1. **UI Implementation** — Add model picker dropdown to select thinking level
2. **Session Persistence** — Save `thinkingLevel` in session header
3. **Cost Hints** — Display tooltips ("High ≈ 6× tokens vs Low")
4. **Per-Turn Override** — Allow `/think <level>` command to switch levels mid-conversation
5. **Extended Provider Support** — Cline and Devin providers (if/when they add reasoning support)

## Commits

1. `docs: Update thinking-levels-spec with Codex and Claude provider details`
2. `feat: Add thinking level support to Codex and Claude models in registry`
3. `feat: Add thinking level support to Codex stream function`
4. `feat: Add thinking level support to Claude stream function`
5. `feat: Add thinking level validation to agent loop`
6. `test: Add comprehensive thinking level tests for Codex, Claude, and validation`

## Verification

✅ TypeScript compilation: No errors (`bunx tsc --noEmit`)
✅ All test suites pass:
  - Validation tests: 24/24 ✓
  - Codex tests: 10/10 ✓
  - Claude tests: 14/14 ✓
  - Total: 48 tests, 0 failures

## Documentation

See `docs/plan/thinking-levels-spec.md` for complete specification including:
- Cross-vendor API details
- Implementation rationale
- Model availability by provider
- Cost considerations
- Best practices
