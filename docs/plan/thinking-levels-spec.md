# Thinking Levels Support Spec

## Summary

Models expose a reasoning dial under different names but the same primitive: how long the model thinks before it speaks. Console should expose it as a single `thinkingLevel` control, normalized per-provider, with model-aware defaults.

## Scope

This implementation supports **backend-only** for three providers:

1. **Antigravity (Gemini)** — ✅ Already implemented
2. **Codex (OpenAI via ChatGPT OAuth)** — 🔄 Implementation in progress
3. **Claude (Anthropic via subscription OAuth)** — 🔄 Implementation in progress

Included:
- Shared backend/types definitions for thinking levels and model capabilities.
- Runtime validation and resolution of a thinking level for each provider request.
- Provider-specific request mapping (Gemini, OpenAI Codex, Anthropic Claude).
- Backend tests for defaults, supported levels, invalid values, and request payloads.

Explicitly deferred:
- Desktop and mobile UI changes.
- Session or per-turn persistence and API changes.
- Cline and Devin provider support.
- Model ID migration or normalization. Existing model IDs remain unchanged.

The backend should use a model's configured default when no level is supplied. Until session/API persistence is introduced, callers may pass an optional runtime level to the agent request path; omitted values must preserve current behavior or use the model's declared default.

## Research: Cross-Vendor Naming (2025-2026)

### OpenAI Codex (`reasoning.effort`)

**Supported levels:** `none | minimal | low | medium | high | xhigh | max`

- **Models:** gpt-5.4-mini, gpt-5.5, gpt-5.6-terra, gpt-5.6-luna, gpt-5.6-sol all support the full range.
- **Parameter:** Request body field `reasoning: { type: "enabled", effort: "..." }` (not a top-level `reasoning_effort`).
- **Default:** Usually `medium`, varies by model.
- **Scale:**
  - `none` — no reasoning
  - `minimal` — minimal reasoning
  - `low` — fast, cost-efficient
  - `medium` — balanced (default for most models)
  - `high` — complex tasks, difficult debugging
  - `xhigh` — deep research, agentic work
  - `max` — maximum capability, highest cost
- **Docs:** [developers.openai.com/api/docs/guides/reasoning](https://developers.openai.com/api/docs/guides/reasoning)

### Anthropic Claude (`output_config.effort`)

**Supported levels:** `low | medium | high | xhigh | max` (**no `none` or `minimal`**)

- **Supported models:** Opus 4.6+, Sonnet 4.6+, Fable 5+, Mythos 5+ (not all models support all levels).
- **Parameter:** Top-level request field `output_config: { effort: "..." }` (replaces deprecated `budget_tokens`).
- **Default:** `high` (equivalent to not setting the parameter).
- **Model availability:**
  - `low`, `medium`, `high`: All Claude models
  - `xhigh`: Opus 4.7+, Fable 5+, Sonnet 5+
  - `max`: Opus 4.7+, Fable 5.1+, Sonnet 5+ only
- **Scale:**
  - `low` — simple, fast, cost-efficient (subagents)
  - `medium` — balanced approach with moderate token savings
  - `high` — default, complex reasoning, difficult coding
  - `xhigh` — long-horizon agentic work (30+ min tasks), extended capability
  - `max` — frontier reasoning, absolute max capability
- **Docs:** [platform.claude.com/docs/en/build-with-claude/effort](https://platform.claude.com/docs/en/build-with-claude/effort)

### Google Gemini via Antigravity (`thinkingConfig.thinkingLevel`)

**Supported levels:** `minimal | low | medium | high` (varies by model)

- Models: Gemini 3.1 Pro, Gemini 3.5+ Flash, Gemini 2.5 (with numeric `thinkingBudget` mapping).
- Parameter: `generationConfig.thinkingConfig.thinkingLevel` (uppercase).
- Default: `high` (most expensive; must set explicitly in requests).
- Already implemented; existing implementation in `apps/server/providers/src/antigravity/`.

## Normalization Mapping

Console uses a unified `ThinkingLevel = "none" | "minimal" | "low" | "medium" | "high" | "xhigh" | "max"` across all providers. Provider adapters map inbound calls to the provider's native format.

| Console `thinkingLevel` | OpenAI Codex | Anthropic Claude | Google Gemini |
|---|---|---|---|
| `none` | `"none"` | ✗ Not supported | ✗ Not supported |
| `minimal` | `"minimal"` | ✗ Not supported | `"minimal"` |
| `low` | `"low"` | `"low"` | `"low"` |
| `medium` | `"medium"` | `"medium"` | `"medium"` |
| `high` | `"high"` | `"high"` | `"high"` |
| `xhigh` | `"xhigh"` | `"xhigh"` | ✗ Not supported |
| `max` | `"max"` | `"max"` | ✗ Not supported |

**Key observations:**
- **Codex** supports all 7 levels (none through max) — the superset.
- **Claude** supports 5 levels (low, medium, high, xhigh, max) — no "none" or "minimal"; omit the parameter to disable thinking.
- **Gemini** supports 4 levels (minimal, low, medium, high) — no extended reasoning levels.
- **Validation rule:** When a Console caller requests an unsupported level, validation rejects it with a clear error message per provider.

## Implementation Details

### 1) Type Definitions

Already defined in `packages/types/src/model.ts`:

```typescript
export type ThinkingLevel = "none" | "minimal" | "low" | "medium" | "high" | "xhigh" | "max";

export interface Model {
  id: string;
  provider: ProviderId;
  contextWindow: number;
  supportsImages?: boolean;
  supportedThinkingLevels?: ThinkingLevel[];   // New field
  defaultThinkingLevel?: ThinkingLevel;        // New field
}
```

### 2) Provider Adapters

**Codex:** `apps/server/providers/src/codex/stream-fn.ts`
- Map `thinkingLevel` to `reasoning.effort` in request body.
- No level filtering; pass through all 7 levels.
- Insert `reasoning: { type: "enabled", effort: mapThinkingLevelToCodex(level) }` in request body.

**Claude:** `apps/server/providers/src/claude/stream-fn.ts`
- Map `thinkingLevel` to `output_config.effort` in request body.
- Filter out "none" and "minimal"; omit the param for thinking-off behavior.
- Insert `output_config: { effort: mapThinkingLevelToClaude(level) }` in request body (at top level).

**Gemini (Antigravity):** `apps/server/providers/src/antigravity/stream-fn.ts`
- Already implemented.
- Map `thinkingLevel` to `generationConfig.thinkingConfig.thinkingLevel` (uppercase).

### 3) UX (Deferred)

Deferred to a later phase:
- Model picker dropdown to select thinking level (visible only if `model.supportedThinkingLevels`).
- Session header persistence of `thinking_level`.
- Desktop composer shortcut (e.g., `Cmd+.`) to switch level per turn.
- Cost hint tooltips ("High ≈ 6× tokens vs Low").

### 4) Backward Compatibility

- Existing sessions with suffixed model IDs (e.g., `gemini-3.1-pro-high`) remain compatible.
- API defaults: If client omits level, server uses `model.defaultThinkingLevel`.

## Backend MVP Implementation Steps

### Step 1: Update Model Registry
**File:** `apps/server/agent/src/commands/provider-registry.ts`

Add `supportedThinkingLevels` and `defaultThinkingLevel` to Codex and Claude model definitions:

```typescript
const CODEX_THINKING_LEVELS: ThinkingLevel[] = [
  "none", "minimal", "low", "medium", "high", "xhigh", "max"
];

const CLAUDE_THINKING_LEVELS: ThinkingLevel[] = [
  "low", "medium", "high", "xhigh", "max"
];

export const DEFAULT_CODEX_MODELS: Model[] = [
  /* ... */
].map((id) => ({
  id,
  provider: "codex",
  contextWindow: 272_000,
  supportsImages: true,
  supportedThinkingLevels: CODEX_THINKING_LEVELS,
  defaultThinkingLevel: "medium" as const,
}));

export const DEFAULT_CLAUDE_MODELS: Model[] = [
  /* ... */
].map(({ id, contextWindow }) => ({
  id,
  provider: "claude",
  contextWindow,
  supportsImages: true,
  supportedThinkingLevels: CLAUDE_THINKING_LEVELS,
  defaultThinkingLevel: "high" as const, // Claude defaults to high
}));
```

### Step 2: Update Codex Stream Function
**File:** `apps/server/providers/src/codex/stream-fn.ts`

Add thinking level mapping:

```typescript
function mapThinkingLevelToCodex(level?: ThinkingLevel): string | undefined {
  if (!level) return undefined;
  return level as string; // Direct 1:1 mapping: "low" → "low", etc.
}

export const codexStreamFn: StreamFn = async (params) => {
  const { thinkingLevel, messages, tools, ...otherParams } = params;
  
  const requestBody = {
    input: convertInput(messages),
    tools: tools.length > 0 ? convertTools(tools) : undefined,
    reasoning: {
      type: "enabled",
      effort: mapThinkingLevelToCodex(thinkingLevel),
    },
    // ... other fields
  };
  // ... rest of stream implementation
};
```

### Step 3: Update Claude Stream Function
**File:** `apps/server/providers/src/claude/stream-fn.ts`

Add thinking level mapping:

```typescript
function mapThinkingLevelToClaude(level?: ThinkingLevel): string | undefined {
  if (!level) return undefined;
  
  // Claude doesn't support "none" or "minimal" — omit the param
  if (level === "none" || level === "minimal") {
    return undefined;
  }
  
  return level as string; // "low" → "low", "high" → "high", etc.
}

export const claudeStreamFn: StreamFn = async (params) => {
  const { thinkingLevel, messages, tools, ...otherParams } = params;
  
  const requestBody = {
    model,
    max_tokens: CLAUDE_MAX_OUTPUT_TOKENS,
    messages: convertClaudeMessages(messages),
    tools: tools.length > 0 ? convertClaudeTools(tools) : undefined,
    output_config: {
      effort: mapThinkingLevelToClaude(thinkingLevel),
    },
    // ... other fields
  };
  // ... rest of stream implementation
};
```

### Step 4: Thread thinkingLevel Through Agent Loop
**File:** `apps/server/agent/src/service/stream-turn.ts`

Ensure `StreamParams` includes and passes `thinkingLevel`:

```typescript
export interface StreamParams {
  // ... existing fields
  thinkingLevel?: ThinkingLevel;
}

// When calling the provider streamFn:
const streamParams: StreamParams = {
  messages,
  tools,
  thinkingLevel, // ← Pass through from agent request
  // ... other params
};

const resultStream = await streamFn(streamParams);
```

### Step 5: Update Provider StreamFn Signatures
**Files:** `apps/server/providers/src/codex/stream-fn.ts` and `claude/stream-fn.ts`

Ensure the exported `streamFn` accepts `thinkingLevel` in its parameters.

### Step 6: Add Validation
**File:** `apps/server/agent/src/service/` (new or extend existing)

```typescript
function validateThinkingLevelForModel(model: Model, level?: ThinkingLevel): void {
  if (!level) return; // Omitted = use default, always valid
  
  if (!model.supportedThinkingLevels?.includes(level)) {
    throw new Error(
      `Model "${model.id}" does not support thinking level "${level}". ` +
      `Supported: ${model.supportedThinkingLevels?.join(", ") || "none"}`
    );
  }
}

// Call before sending to provider:
validateThinkingLevelForModel(selectedModel, requestedThinkingLevel);
```

### Step 7: Backend Tests
**File:** `apps/server/tests/` (new test files)

Create tests for:
- Codex: Mapping all 7 levels to `reasoning.effort`
- Claude: Mapping supported 5 levels, rejecting "none"/"minimal"
- Unsupported levels are rejected per provider
- Omitted levels resolve to model defaults
- Generated request payloads are correct for each level

## Backend MVP Verification

✅ Codex request payloads contain the expected `reasoning.effort` value.  
✅ Claude request payloads contain the expected `output_config.effort` value.  
✅ Unsupported levels are rejected before the provider request is sent (with clear error).  
✅ Omitted levels resolve to the registered model default.  
✅ Existing model IDs and request behavior remain compatible when no level is supplied.  
✅ Run focused server/provider tests and `bunx tsc --noEmit`.  

## Backend MVP Risks

- **Model-dependent valid sets:** Validate per provider and reject unsupported levels with clear error messages.
- **Codex vs. Claude parameter placement:** Codex uses nested `reasoning.effort`; Claude uses top-level `output_config.effort`. Different insertion points in request body.
- **Claude thinking-off behavior:** No "none"/"minimal" support; omit the `effort` parameter to disable thinking entirely (do not pass empty string).
- **Cost trap on defaults:** Claude defaults to `high` (most expensive); Codex defaults to `medium`. Explicitly configure safe defaults in model registry.

## Next Steps (Future Phases)

1. **Phase 2 (UI):** Add model picker dropdown and session persistence for thinking level.
2. **Phase 3 (UX Polish):** Cost hint tooltips, per-turn thinking level override via slash command or menu.
3. **Phase 4 (Extended Support):** Cline and Devin provider support (if those models expose thinking-level equivalents).
