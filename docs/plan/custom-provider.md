# Custom Provider Implementation Guide

Adds user-defined LLM endpoints to the server so any **OpenAI-compatible** or
**Anthropic-compatible** API (OpenRouter, aggregators, proxies, self-hosted
gateways) works as a first-class provider with multiple saved endpoint profiles.

Target layout: `apps/server/providers/src/custom/`

## Core concepts

- **Profile** — one saved endpoint:
  ```ts
  interface CustomProviderProfile {
    id: string;              // stable uuid
    label: string;           // user-facing name, e.g. "OpenRouter main"
    kind: "openai-compatible" | "anthropic-compatible";
    baseUrl: string;         // e.g. https://openrouter.ai/api/v1
    apiKey?: string;         // stored via token-store pattern, never returned raw
    createdAt: number;
  }
  ```
- Profiles are stored under the console storage dir (`~/.console`) alongside
  other provider credentials (`providers/src/auth/token-store.ts` pattern).
- A profile is addressable as a provider: `provider = "custom:<profileId>"`.
- Primary live target for verification: **OpenRouter** (`https://openrouter.ai/api/v1`,
  `Authorization: Bearer sk-or-…`, model ids like `vendor/model`, optional
  `HTTP-Referer` / `X-Title` headers).

## Non-negotiable gotchas (learned from opencode)

1. Tools passed to `streamText` must be wrapped with the SDK's `tool()` helper
   carrying the original zod schema under `inputSchema:`. Raw JSON Schema under
   `parameters:` (v4 convention) makes `asSchema(undefined)` substitute
   `{ properties: {}, additionalProperties: false }` — every parameterized call
   then fails validation. See `opencode/convert-tools.ts` post-fix.
2. Never await the stream inside WS open handlers; fragment accumulation for
   streamed tool-call arguments already exists in the agent loop.
3. API keys must never round-trip to clients in full — mask on read.

---

## Phase 1 — Profile storage & config service

### Tasks

1. Create `custom/profiles.ts`:
   - `CustomProviderProfile` type (above).
   - `listProfiles()`, `getProfile(id)`, `saveProfile(input)`, `deleteProfile(id)`
     persisted as JSON under `<storageDir>/custom-providers.json`.
   - `maskProfile(profile)` → replaces `apiKey` with `sk-…<last4>` for client responses.
2. Add `kind` union export shared by both engines.
3. Unit tests: CRUD round-trip, masked output, delete of unknown id.

### Acceptance

- [ ] Profiles survive daemon restart.
- [ ] No API surface returns a full apiKey.

---

## Phase 2 — OpenAI-compatible engine

### Tasks

1. `custom/openai-compatible/index.ts`:
   - `createCustomOpenAIStreamFn(profile)` → StreamFn using
     `createOpenAICompatible({ name: profile.id, baseURL: profile.baseUrl, apiKey: profile.apiKey })`
     + `streamText` (clone of `opencode/stream-fn.ts` minus free-tier specifics).
2. `custom/shared/convert-tools.ts`:
   - `tool()`-wrapped ToolSet builder keyed off zod schemas (shared by both engines;
     anthropic engine imports this too where applicable).
3. `custom/openai-compatible/convert-messages.ts`:
   - Start by reusing `opencode/convert-messages.ts`; extract to
     `custom/shared/convert-messages.ts` if identical.
4. `custom/openai-compatible/discovery.ts`:
   - `GET {baseUrl}/models` with `Authorization: Bearer <key>`.
   - Map response `{ data: [{ id }] }` → provider catalog entries.
   - On failure return empty list + reason (server down / 401 / not supported).
5. Error handling: connection refused, 401/403, non-JSON body, timeouts → typed
   errors that map to clean SSE/error frames instead of HTTP 500.

### Acceptance

- [ ] Live OpenRouter run: chat turn streams text.
- [ ] Live OpenRouter run with a tool-capable model executes `readFile` end-to-end.
- [ ] `/models` listing populates the picker.

---

## Phase 3 — Anthropic-compatible engine

Anthropic's Messages API differs structurally (top-level `system`, `x-api-key`
+ `anthropic-version` headers, `input_schema` tool fields, distinct event stream).

### Tasks

1. Add dependency `@ai-sdk/anthropic` (same major family as `ai@7`).
2. `custom/anthropic-compatible/index.ts`:
   - `createCustomAnthropicStreamFn(profile)` → StreamFn using
     `createAnthropic({ baseURL: profile.baseUrl, apiKey: profile.apiKey })`.
3. `custom/anthropic-compatible/discovery.ts`:
   - `GET {baseUrl}/v1/models` with `x-api-key` + `anthropic-version` headers.
4. Message conversion: verify AI SDK maps our UIMessages correctly through the
   anthropic provider; add converter only if gaps appear (e.g. image parts,
   thinking blocks).
5. Same error taxonomy as Phase 2 step 5.

### Acceptance

- [ ] Run against an Anthropic-compatible endpoint streams text + tool calls.
- [ ] `/models` listing works with `x-api-key`.

---

## Phase 4 — Registry & API surface

### Tasks

1. `agent/src/commands/provider-registry.ts`:
   - Expose each enabled profile as a provider entry with
     `id: "custom:<profileId>"`, `authMethod: "apiKey"`, and its discovered models.
2. `run.service.ts`:
   - Resolve `custom:<profileId>` to the right StreamFn factory at run time
     (factory is cheap; construct per-run from the stored profile).
3. REST routes under `api/src/routes/providers.ts` (or new `custom-providers.ts`):
   - `GET    /api/providers/custom`          → masked list
   - `POST   /api/providers/custom`          → create (validate baseUrl URL)
   - `PATCH  /api/providers/custom/:id`      → update (label/key/models)
   - `DELETE /api/providers/custom/:id`
   - `POST   /api/providers/custom/:id/test` → connectivity + models probe
4. Permission/approval flow: unchanged — custom profiles are just another provider.

### Acceptance

- [ ] Mobile/desktop account screens can list/create/edit/delete profiles.
- [ ] A run addressed to `custom:<profileId>` completes against OpenRouter.
- [ ] Deleting a profile mid-session fails gracefully on next run.

---

## Phase 5 — Tests & hardening

### Tasks

1. `tests/custom-openai.test.ts` — mirror `opencode.test.ts`: mocked fetch,
   wire-schema assertions (real properties present — regression-guard the v4/v7
   pitfall), streamed toolCall accumulation.
2. `tests/custom-anthropic.test.ts` — same shape for the anthropic engine.
3. Profile service tests (Phase 1) wired into `tests/run-all-tests.ts`.
4. Docs: note OpenRouter specifics in README or provider docs (optional headers,
   model-id format).

### Acceptance

- [ ] All new suites pass via `bun tests/<file>.test.ts`.
- [ ] Roadmap updated (custom providers item checked).

---

## Deliberately out of scope (v1)

- OAuth flows for custom endpoints (static API keys only)
- Per-profile tool allowlists/denylists
- Streaming usage/cost accounting (OpenRouter reports usage — candidate for v2)
- Aliasing actual first-party OpenAI/Anthropic accounts through these engines
