# Thinking Levels Support Spec

## Summary
Models expose a reasoning dial under different names but the same primitive: how long the model thinks before it speaks. Console should expose it as a single `thinkingLevel` / `reasoningEffort` control, normalized per-provider, with model-aware defaults.

## Current Scope
This first implementation is **backend-only** and supports **Antigravity/Gemini models only**.

Included:
- Shared backend/types definitions for thinking levels and model capabilities.
- Runtime validation and resolution of a thinking level for each Antigravity request.
- Antigravity request mapping through Gemini `generationConfig.thinkingConfig`.
- Backend tests for defaults, supported levels, invalid values, and request payloads.

Explicitly deferred:
- Desktop and mobile UI changes.
- Session or per-turn persistence and API changes.
- OpenCode, Codex, Cline, Devin, Anthropic, and direct OpenAI provider support.
- Model ID migration or normalization. Existing model IDs remain unchanged.
- Gemini numeric `thinkingBudget` compatibility unless required by a currently registered Antigravity model.

The backend should use a model's configured default when no level is supplied. Until session/API persistence is introduced, callers may pass an optional runtime level to the agent request path; omitted values must preserve current behavior or use the model's declared default.

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

> **Backend MVP note:** only the Gemini/Antigravity column is in scope initially. Other provider mappings are design references only and must not be implemented in this phase.
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

## Backend MVP Implementation Steps
1. `packages/types/src/model.ts` — add `ThinkingLevel`, `supportedThinkingLevels`, and `defaultThinkingLevel` to shared model metadata.
2. `apps/server/agent/src/service/types.ts` and agent construction — carry an optional runtime thinking level through the backend stream request path.
3. `apps/server/agent/src/commands/provider-registry.ts` — populate Antigravity/Gemini capabilities and defaults without changing existing model IDs.
4. `apps/server/providers/src/antigravity/stream-fn.ts` — map the resolved level to Gemini `generationConfig.thinkingConfig.thinkingLevel` and validate unsupported values.
5. Add backend-only tests for capability resolution and the generated Antigravity request payload.

Do not modify desktop, mobile, session persistence, frontend APIs, or non-Antigravity providers in this phase.

## Backend MVP Verification
* Antigravity request payloads contain the expected Gemini thinking configuration for each supported level.
* Unsupported levels are rejected before the provider request is sent.
* Omitted levels resolve to the registered Antigravity model default.
* Existing Antigravity model IDs and request behavior remain compatible when no level is supplied.
* Run the focused server/provider tests and `bunx tsc --noEmit`.

## Backend MVP Risks
* Model-dependent valid sets — validate per Antigravity model and reject unsupported levels with a clear error.
* Existing Antigravity model IDs may encode a level suffix — preserve those IDs and aliases in this phase; do not migrate them yet.
* Gemini models may use either named `thinkingLevel` or numeric `thinkingBudget` — do not send both, and defer numeric-budget models until one is registered.
* Cost trap on Gemini defaults — explicitly configure a safe model default rather than assuming the provider default is inexpensive.
