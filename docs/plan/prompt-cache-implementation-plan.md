# Prompt Cache Integration Plan

## 1. Goal

Add provider-aware prompt-cache support to the Console harness before changing compaction. The first phase should make cache behavior intentional, stable, and observable without changing compaction boundaries or retry behavior.

The initial success criteria are:

- Repeated turns in one agent session preserve a stable provider/cache identity.
- Providers receive the cache controls they actually support.
- Assistant usage records expose uncached input, cache reads, cache writes, output, reasoning, and total tokens.
- Cache hits and misses are visible in telemetry and tests.
- Unsupported providers continue working with caching disabled or provider defaults.
- The implementation does not claim that a session ID alone is a prompt cache.

Reference implementations reviewed:

- `/Users/adelodunpeter/Developer/Projects/pi/packages/ai/src/types.ts`
- `/Users/adelodunpeter/Developer/Projects/pi/packages/ai/src/providers/google-shared.ts`
- `/Users/adelodunpeter/Developer/Projects/pi/packages/ai/src/providers/anthropic.ts`
- `/Users/adelodunpeter/Developer/Projects/pi/packages/ai/src/providers/openai-completions.ts`
- `/Users/adelodunpeter/Developer/Projects/pi/packages/ai/src/providers/openai-responses.ts`
- `/Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/catalog/src/model-tokenizer.ts`
- `/Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/catalog/src/build.ts`

## 2. Current Console Gaps

### 2.1 Request options do not contain cache policy

`apps/server/agent/src/service/types.ts` currently passes model, system prompt, messages, tools, signal, and thinking level to `StreamFn`. There is no cache retention or cache identity field.

### 2.2 Antigravity has a stable session envelope, but no cache contract

`apps/server/providers/src/antigravity/stream-fn.ts` creates stable `sessionState` and sends `sessionId`, `requestId`, `trajectoryId`, and step metadata. This may provide backend affinity, but it is not proof that prompt tokens are cached. Cache usage must be confirmed from provider response metadata.

### 2.3 Usage is declared but discarded

`apps/server/providers/src/types/cca.ts` defines `CcaUsageMetadata` with prompt, candidate, and total counts, but it currently omits cached-content and reasoning counts. `apps/server/providers/src/shared/stream-core.ts` parses response chunks and emits only text/tool deltas, so usage never reaches `AssistantMessage` or compaction.

### 2.4 Compaction currently rewrites history without cache awareness

`apps/server/agent/src/compaction/index.ts` remains unchanged in this phase. Cache behavior must be measured first; compaction changes happen only after cache instrumentation and provider support are verified.

## 3. Cache Model

### 3.1 Common options

Add provider-neutral fields to the stream request:

```ts
export type CacheRetention = "short" | "long" | "none";

interface StreamParams {
  model: Model;
  systemPrompt: string;
  messages: AgentMessage[];
  tools: AgentTool[];
  signal?: AbortSignal;
  thinkingLevel?: ThinkingLevel;
  cacheRetention?: CacheRetention;
  sessionId?: string;
}
```

Use `"short"` as the default only where the provider supports an automatic/default cache. Use `"none"` for providers that cannot safely accept cache controls.

### 3.2 Cache identity

- Give each logical agent conversation one stable cache/session identity.
- Reuse it across turns and safe retries.
- Do not generate a new ID for every model request.
- Do not reuse an identity across different users, credentials, models, providers, or incompatible system/tool configurations.
- Include provider and canonical model identity in the cache-key derivation or identity scope.
- Keep compaction-generated context changes observable as a cache-prefix change; do not attempt to pretend the old cache remains valid.

## 4. Provider Strategy

### 4.1 Antigravity / CCA

Phase 1 for Antigravity is observability and stable affinity, not speculative cache fields:

1. Preserve the current stable `AntigravitySessionState` for one `Agent` instance.
2. Add normalized usage propagation from CCA responses.
3. Capture `promptTokenCount`, `cachedContentTokenCount` when present, `candidatesTokenCount`, `thoughtsTokenCount` when present, and `totalTokenCount`.
4. Only add an explicit CCA cache resource/control after a recorded request/response confirms the endpoint supports it. Do not send standard Gemini `cachedContent` blindly to the CCA envelope.
5. If CCA reports no cache fields, record `cacheStatus: "unknown"`, not `cacheRead: 0` as proof of a miss.

### 4.2 Standard Gemini / Vertex

For standard Gemini APIs, use the official cached-content mechanism when the provider adapter supports it:

- Cache stable system instructions, tool definitions, and other immutable prefix content.
- Send only compatible request-level fields when `cachedContent` is supplied.
- Respect the provider restriction that cached content cannot be combined arbitrarily with request-level system instructions, tools, or tool configuration.
- Normalize usage as:

```text
uncachedInput = promptTokenCount - cachedContentTokenCount
cacheRead = cachedContentTokenCount
output = candidatesTokenCount + thoughtsTokenCount
```

Reference: `/Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/ai/src/providers/google-shared.ts`.

### 4.3 Anthropic

When using an Anthropic-compatible adapter:

- Add cache markers to stable system prompt content.
- Add a marker to the stable tool definition boundary.
- Add a marker to the last stable user/assistant/tool content boundary.
- Do not attach cache markers to generated reasoning blocks when the provider rejects them.
- Read `cache_read_input_tokens` and `cache_creation_input_tokens` from usage.
- Preserve 5-minute versus 1-hour cache-write distinctions if the provider reports TTL-specific values.

The old `@anthropic-ai/tokenizer` package is not part of this implementation; this phase uses provider usage, not a local tokenizer.

### 4.4 OpenAI-compatible providers

When supported by the endpoint/model compatibility metadata:

- Send a stable `prompt_cache_key` derived from the logical session identity.
- Send `prompt_cache_retention` only for providers that document it.
- Apply Anthropic-style `cache_control` only when the compatibility record explicitly supports that format.
- Preserve cache usage fields returned by the provider.

Do not add generic fields to every OpenAI-compatible request because gateways vary in their support.

### 4.5 Unsupported providers

- Omit cache controls.
- Keep usage fields at zero only when the provider explicitly reports no cache usage; otherwise use an `unknown` cache status internally.
- Never fail a request solely because optional cache controls are unavailable.

## 5. Usage Data Contract

### 5.1 Normalized usage

Extend the internal assistant usage model with optional normalized fields as needed:

```ts
interface Usage {
  input: number;          // uncached input
  cacheRead: number;
  cacheWrite: number;
  output: number;
  reasoningTokens?: number;
  totalTokens: number;
  cacheStatus?: "hit" | "miss" | "write" | "unknown" | "unsupported";
}
```

The existing cost calculation must use uncached input separately from cache-read and cache-write tokens so cached tokens are not double-counted.

### 5.2 Antigravity usage propagation

Update:

- `apps/server/providers/src/types/cca.ts`
- `apps/server/providers/src/shared/stream-core.ts`
- `apps/server/agent/src/service/stream-turn.ts`
- `apps/server/agent/src/types/message.ts`

Preferred approach: let `streamCore` return or attach a final usage object to the stream result, then have `streamOneTurn` attach it to the final `AssistantMessage`. Avoid emitting usage once per response chunk unless the provider only supplies cumulative usage in deltas; the final cumulative record is authoritative.

### 5.3 Usage and cache telemetry

Add non-sensitive telemetry fields:

- Provider and model.
- `inputTokens`.
- `cacheReadTokens`.
- `cacheWriteTokens`.
- `outputTokens`.
- `reasoningTokens`.
- `totalTokens`.
- `cacheStatus`.
- Cache retention mode.
- Stable cache identity fingerprint, never the raw session ID.
- Whether the request followed a retry or compaction.

Never log prompts, tool contents, credentials, or raw cache keys.

## 6. Stable-Prefix Rules

The cache only helps when the request prefix remains identical. Establish these rules before compaction work:

- Keep the system prompt byte-for-byte stable across turns unless the workspace context genuinely changes.
- Keep tool ordering and serialized schemas deterministic.
- Keep tool names, descriptions, and schema property ordering deterministic.
- Keep the same canonical model/provider route for a cache identity.
- Append new conversation content rather than rewriting old content.
- Do not add timestamps, random IDs, or per-request noise to cached prefix content.
- Keep request IDs and transport tracing metadata outside the prompt content where possible.
- Treat a compaction summary as a new prefix generation and record the resulting cache reset/miss.

## 7. Implementation Steps

### Step 1: Define cache-neutral types and identity

Files:

- `apps/server/agent/src/service/types.ts`
- `apps/server/agent/src/service/agent.ts`
- `apps/server/agent/src/types/message.ts`
- `apps/server/providers/src/types/index.ts`

Tasks:

1. Add `CacheRetention`, cache status, normalized usage fields, and optional session/cache identity fields.
2. Create a stable identity once per logical `Agent` instance.
3. Pass cache options through `StreamFn` without provider-specific fields.
4. Add a bounded identity/fingerprint helper for telemetry.

### Step 2: Propagate provider usage

Files:

- `apps/server/providers/src/types/cca.ts`
- `apps/server/providers/src/shared/stream-core.ts`
- `apps/server/agent/src/service/stream-turn.ts`
- `apps/server/agent/src/types/message.ts`

Tasks:

1. Extend CCA usage types with cached and reasoning token fields.
2. Capture final cumulative usage from CCA SSE chunks.
3. Attach normalized usage to the assistant message.
4. Preserve usage through persistence and session continuation.
5. Add tests for cache-read, cache-write, reasoning, and total-token normalization.

### Step 3: Add Antigravity cache observability

Files:

- `apps/server/providers/src/antigravity/stream-fn.ts`
- `apps/server/providers/src/shared/stream-core.ts`
- `apps/server/tests/antigravity-thinking.test.ts` or a dedicated cache test.

Tasks:

1. Pass stable session/cache identity through the provider call.
2. Verify whether CCA returns cache metadata in production-shaped responses.
3. Record `unknown` rather than assuming misses when metadata is absent.
4. Do not add unsupported `cachedContent` fields until verified.

### Step 4: Implement standard provider cache adapters

Files depend on the existing provider surface:

- Gemini/Vertex provider adapter.
- Anthropic provider adapter.
- OpenAI completions/responses adapters.
- Provider compatibility metadata.

Tasks:

1. Map neutral options to provider-supported controls.
2. Make stable system/tool/content cache boundaries deterministic.
3. Add provider-specific response usage normalization.
4. Keep unsupported-provider behavior unchanged.

### Step 5: Add observability and cost validation

Tasks:

1. Emit normalized cache usage on assistant completion.
2. Confirm cost calculation does not count cache reads as uncached input.
3. Compare repeated-turn usage across cache-enabled and cache-disabled runs.
4. Record cache hit/miss/write rates and prefix invalidation events.

### Step 6: Only after cache validation, update compaction

Do not implement tokenizer-aware compaction or new compaction boundaries in this phase. Once cache usage is visible:

- Use provider usage as the measured context anchor.
- Treat compaction as a deliberate cache-prefix reset.
- Avoid unnecessary compaction rewrites.
- Integrate tokenizer/fallback estimation for only the unmeasured tail.

The follow-up work belongs in `docs/plan/context-overflow-compaction-resilience-plan.md`.

## 8. Verification Plan

### Unit tests

- Stable identity is reused across turns and safe retries.
- Identity changes when provider/model changes.
- System/tool serialization is deterministic.
- CCA usage normalizes cached and reasoning tokens correctly.
- Missing CCA cache metadata results in `unknown`, not a false cache miss.
- Anthropic cache-read and cache-write usage is normalized correctly.
- OpenAI cache controls are omitted for unsupported compatibility records.
- Cache reads are not double-counted as normal input in costs.

### Integration tests

- Two identical-prefix requests produce a cache hit when the provider supports it.
- Adding a new user/tool-result suffix preserves the stable prefix cache where supported.
- Changing system prompt or tool schema records a cache invalidation.
- Compaction produces an explicit prefix-generation/cache-reset event.
- Provider retries reuse cache identity when safe.
- Unsupported providers still complete normally.

### Production validation

Capture aggregate metrics for a representative session:

```text
request number
provider/model
input tokens
cache-read tokens
cache-write tokens
output tokens
reasoning tokens
cache status
prefix generation
compaction/retry flags
```

Success means the team can explain whether higher usage comes from cache misses, cache writes, reasoning output, tool growth, or compaction resets.

## 9. Non-Goals

- This plan does not solve context compaction by itself.
- This plan does not add a universal tokenizer.
- This plan does not assume Antigravity supports standard Gemini cached-content resources.
- This plan does not cache arbitrary mutable tool output.
- This plan does not expose raw prompts or cache keys in logs.
