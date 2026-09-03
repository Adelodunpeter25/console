# Thinking Levels Support Spec

## Summary
Models expose a reasoning dial under different names but the same primitive: how long the model thinks before it speaks. Console should expose it as a single `thinkingLevel` / `reasoningEffort` control, normalized per-provider, with model-aware defaults and per-session persistence.

## Research

### Cross-vendor naming (2025-2026)
* **OpenAI** (`reasoning.effort` / `reasoning_effort`): `none | minimal | low | medium | high | xhigh | max` (model-dependent). `gpt-5.5` defaults `medium`; `gpt-5.1` supports `none,low,medium,high`; `gpt-5-pro` only `high`; `o3-mini` supports `low|medium|high` via `reasoning_effort`. Lower = fewer reasoning tokens, faster, less thorough. `xhigh`/`max` only after `gpt-5.1-codex-max` / `gpt-6`. See `providers/src/opencode/stream-fn.ts:82` `reasoning-delta` handling.
* **Anthropic Claude** (`effort` + `extended_thinking`): `low | medium | high | max` (some models also `xhigh`). Old `budget_tokens` fixed budget is deprecated → adaptive `effort` behavioral signal, not strict token cap. Opus 4.8 uses adaptive thinking, no manual `budget_tokens` (400 error). Sonnet 4.6 / Opus 4.x default `high` (app) or `medium` (API). Scale: Low~1k, Medium~2.5k, High~6k, Max~12k+ tokens (`explainx.ai/blog/claude-effort-parameter`).
* **Google Gemini** (`thinkingConfig.thinkingLevel` / `generationConfig.thinkingConfig`): Gemini 3.1 Pro: `low | medium | high` (default `high` → most expensive, must set explicitly). Gemini 3.5/3.6/3.8 Flash: `minimal | low | medium | high` (3.8 Flash defaults `medium`, 3.1 Flash-Lite defaults `minimal`). Gemini 2.5: `thinkingBudget` numeric (0-24576) → map 0-1024→`low`, 1024-8192→`medium`, >8192→`high`; cannot set both `thinkingBudget` + `thinkingLevel` (400).
* **xAI / DeepSeek**: `reasoning_mode` toggle or internal COT — no granular levels; treat as binary.

Console already streams `thinking` deltas (`apps/server/agent/src/service/stream-turn.ts:52`, `thinking.ts:14` `<thinking>` parsing, `packages/types/src/events.ts:82` `part.thinking`, `providers/src/opencode/convert-messages.ts:42` `reasoning`).

Current registry already has level-like model IDs but not a runtime dial: `apps/server/agent/src/commands/provider-registry.ts:41` `gemini-3.1-pro-high`, `gemini-3.1-pro-low`, `gpt-oss-120b-medium`, etc. — suffix mirrors effort. No `thinkingLevel` field on `Model` or `SessionHeader`.

### When level matters
* **Planning/analysis, hard debugging, architecture** → `high`/`xhigh`. AIME 2026 low→high +18-22pp, GPQA +3-7pp (`aipatternbook.com/reasoning-effort`).
* **Mechanical execution** (format, boilerplate, apply spec) → `minimal`/`low` — extra thinking regresses Expert-SWE (medium beats high by 3-5pp).
* **Default for code** → `medium` (balanced). Don't default Gemini 3.1 Pro to `high` without override (cost trap).

## Proposed Console Model

### 1) Data model
```ts
// @console/types/src/model.ts
export type ThinkingLevel = "none" | "minimal" | "low" | "medium" | "high" | "xhigh" | "max";
export interface Model {
  id: string;
  provider: ProviderId;
  contextWindow: number;
  supportsImages?: boolean;
  supportedThinkingLevels?: ThinkingLevel[]; // if absent, model doesn't support dial
  defaultThinkingLevel?: ThinkingLevel;
}
```
```ts
// Session persistence: session_meta + sessions table
thinking_level?: ThinkingLevel | null  // null = use model default
```

### 2) Provider mapping
| Console `ThinkingLevel` | OpenAI `reasoning.effort` | Anthropic `effort` | Gemini `thinkingLevel` |
|---|---|---|---|
| `none` | `none` | — (thinking off) | — |
| `minimal` | `minimal` | — | `MINIMAL` |
| `low` | `low` | `low` | `LOW` |
| `medium` | `medium` | `medium` | `MEDIUM` |
| `high` | `high` | `high` | `HIGH` |
| `xhigh` | `xhigh` | `xhigh` | — |
| `max` | `max` | `max` | — |

Provider adapters:
* `apps/server/providers/src/opencode/stream-fn.ts` / `providers/src/openai-compatible` — map to `reasoning.effort` in request body.
* `apps/server/providers/src/anthropic` — map to `effort`, drop `budget_tokens` handling (keep backward read).
* `apps/server/providers/src/gemini` (new or inside antigravity) — map to `generationConfig.thinkingConfig.thinkingLevel` (upper-case). Enforce not sending `thinkingBudget` simultaneously.
* Registry `DEFAULT_ANTIGRAVITY_MODELS` can split `gemini-3.1-pro-high` into base `gemini-3.1-pro` + levels instead of separate IDs — migrate IDs but keep aliases for compat.

### 3) UX
* Model picker (`apps/desktop/src/state/app.ts`, `apps/desktop/crates/console-ui/src/inspector`) — second dropdown `Thinking` visible only if `model.supportedThinkingLevels`. Default shows `Medium (default)`; hide for models without dial.
* Session header: persist `thinking_level`; `updateSession` (`apps/server/api/src/services/session.service.ts:83`) accepts `thinkingLevel`. Inspector shows badge “Thinking: High”.
* Desktop composer: `Cmd+.` or `⋮` menu to switch level per turn without changing model (like `apps/desktop/src/view/workspace_content.rs` model selector).
* Cost hint: tooltip “High ≈ 6× tokens vs Low”.

### 4) Backward compat
* Existing sessions with suffixed IDs (`gemini-3.1-pro-high`) auto-migrate: parse suffix → set `thinkingLevel`, normalize ID to base.
* API defaults: if client omits level, server uses `model.defaultThinkingLevel` (avoid Gemini high-cost default surprise).

## Implementation Steps
1. `packages/types/src/model.ts` + `packages/types/src/session.ts` + `apps/desktop/crates/console-core/src/types` — add `ThinkingLevel`, `supportedThinkingLevels`, `thinking_level`.
2. `apps/server/agent/src/commands/provider-registry.ts` — populate `supportedThinkingLevels`/`defaultThinkingLevel` per model/provider.
3. `apps/server/agent/src/session/schema.ts` + `session-ops.ts` — add `thinking_level` column, migration, `updateThinkingLevel`.
4. `apps/server/agent/src/service/agent-loop.ts` + providers — pass `thinkingLevel` into streamFn request.
5. `apps/server/api/src/routes/sessions.ts` + `services/session.service.ts` — accept/return `thinkingLevel`.
6. `apps/desktop/src/state/*` + `console-ui` — picker, persistence, title/inspector updates.
7. Docs: update `docs/plan/custom-provider.md` wire protocol with `thinkingLevel`.

## Verification
* `gpt-5.5` low vs high: second call shows more reasoning tokens, slower.
* `gemini-3.1-pro` with `low` vs `high` (Deep Think Mini): high cost/latency higher.
* `claude-opus-4-8` high vs max: max uses more tokens on hard task.
* `cargo check` + `bun tests/terminal.test.ts` + manual model picker round-trip.

## Risks
* Model-dependent valid sets — validate per-model, reject `xhigh` on `gptoai` with 400 → user sees error. Gate UI to only valid options.
* Cost trap on Gemini default high — ensure console default overrides to `medium` unless user picks high.
