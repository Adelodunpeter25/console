# Custom Provider Implementation Guide

Adds user-defined LLM endpoints to the server so any **OpenAI-compatible** or
**Anthropic-compatible** API (OpenRouter, DeepSeek, Together, Groq, Ollama, vLLM,
corporate proxies) works as a first-class provider with saved endpoint profiles.

Target layout: `apps/server/providers/src/custom/`

---

## 1. Core Concepts

### 1.1 Profile Model
```ts
export interface CustomModelConfig {
  id: string;              // e.g. "anthropic/claude-3.7-sonnet" or "deepseek/deepseek-r1"
  name?: string;           // Display name
  contextWindow?: number;  // Default: 128_000
  supportsImages?: boolean;// Default: true
}

export interface CustomProviderProfile {
  id: string;              // Stable UUID
  label: string;           // User-facing label, e.g. "OpenRouter Main" or "Local Ollama"
  kind: "openai-compatible" | "anthropic-compatible";
  baseUrl: string;         // e.g. "https://openrouter.ai/api/v1"
  apiKey?: string;         // Stored in token store, masked on read
  models?: CustomModelConfig[]; // Discovered or manually configured models
  defaultModelId?: string;
  createdAt: number;
  updatedAt: number;
}
```

- **Persistence**: Saved under `<storageDir>/custom-providers.json` (mirroring `token-store.ts`).
- **Addressability**: `provider = "custom:<profileId>"`.
- **Primary Live Verification**: **OpenRouter** (`https://openrouter.ai/api/v1`, Bearer auth, `vendor/model` format, `HTTP-Referer` / `X-Title` headers).

---

## 2. Non-Negotiable Gotchas & Invariants

1. **Tool Schema Convention**:
   Tools passed to `streamText` must be wrapped with AI SDK's `tool()` helper with the Zod schema under `inputSchema:`. Passing raw JSON Schema under `parameters:` causes `asSchema(undefined)` to collapse fields into `{ properties: {}, additionalProperties: false }`, failing all parameterized tool calls.
2. **Never Expose Raw API Keys**:
   `apiKey` must be masked on read (`sk-…<last4>`). Only the internal runtime stream factory loads the plain text secret.
3. **OpenRouter Headers**:
   When `baseUrl` matches `openrouter.ai`, include:
   - `HTTP-Referer`: `https://github.com/Adelodunpeter25/console`
   - `X-Title`: `Console Assistant`
4. **Fallback When `/models` Is Disabled**:
   Certain private proxies, Ollama setups, or gateways disable `GET /models` or require separate auth. The profile allows explicit `models` overrides so a user can type a model ID directly.
5. **Zero Client Breaking Changes**:
   Exposing custom profiles dynamically through `listProviders()` in `apps/server/agent/src/commands/provider-registry.ts` means the existing model picker on mobile (`ModelPickerSheet`) and desktop (`right_sidebar.rs` / status bar) will automatically populate custom providers without requiring UI changes.

---

## 3. Phased Implementation Plan

### Phase 1 — Profile Storage & Config Service (Fast Track)

#### Tasks
1. `apps/server/providers/src/custom/profiles.ts`:
   - Define `CustomProviderProfile`, `CustomModelConfig`.
   - `loadProfiles()`, `getProfile(id)`, `saveProfile(input)`, `deleteProfile(id)`.
   - File path: path joined to console storage dir (`<storageDir>/custom-providers.json`).
   - `maskProfile(profile)`: replaces `apiKey` with `sk-…<last4>`.
2. CRUD Validation:
   - Validate `baseUrl` is a valid URL (`http://` or `https://`).
   - Ensure `label` is non-empty.
3. Tests: `apps/server/tests/custom-profile.test.ts` (CRUD, key masking, persistence across re-instantiation).

---

### Phase 2 — OpenAI-Compatible Engine & Discovery (OpenRouter Target)

#### Tasks
1. `apps/server/providers/src/custom/shared/convert-tools.ts`:
   - Zod-based `tool()` wrapper (reused by both OpenAI and Anthropic engines).
2. `apps/server/providers/src/custom/shared/convert-messages.ts`:
   - Standard AI SDK CoreMessage converter supporting text, images, thinking, and tool results.
3. `apps/server/providers/src/custom/openai-compatible/discovery.ts`:
   - `fetchCustomOpenAIModels(profile)`:
     - `GET {baseUrl}/models` with `Authorization: Bearer <key>`.
     - Maps `{ data: [{ id, name, context_length }] }` to `CustomModelConfig[]`.
     - Graceful fallback on error (returns profile's manual `models` list or generic fallback).
4. `apps/server/providers/src/custom/openai-compatible/stream-fn.ts`:
   - `createCustomOpenAIStreamFn(profile)`:
     - Uses `createOpenAICompatible({ name: profile.id, baseURL: profile.baseUrl, apiKey: profile.apiKey, headers: profile.isOpenerRouter ? ... : {} })`.
     - Calls `streamText` yielding text deltas, thinking deltas, and tool calls.
5. Error mapping:
   - 401 Unauthorized -> "Invalid custom provider API key".
   - 404 / connection refused -> "Custom provider unreachable at {baseUrl}".

---

### Phase 3 — Dynamic Registry & Provider Integration

#### Tasks
1. `apps/server/agent/src/commands/provider-registry.ts`:
   - Import `listProfiles` from `custom/profiles.js`.
   - In `getProvider(id)`:
     - If `id.startsWith("custom:")`, parse `profileId = id.slice(7)`.
     - Load profile and instantiate StreamFn on the fly.
   - In `listProviders()`:
     - Append custom profiles as active providers:
       ```ts
       {
         id: `custom:${profile.id}`,
         name: profile.label,
         authMethod: "apiKey",
         models: profile.models && profile.models.length > 0
           ? profile.models.map(m => ({ id: m.id, provider: `custom:${profile.id}`, contextWindow: m.contextWindow ?? 128_000, supportsImages: m.supportsImages ?? true }))
           : [{ id: "default", provider: `custom:${profile.id}`, contextWindow: 128_000 }]
       }
       ```
2. `apps/server/api/src/services/run.service.ts`:
   - Ensure `buildRunModel` handles dynamic `custom:*` provider prefixes safely.

---

### Phase 4 — REST API Endpoints

#### Tasks
Under `apps/server/api/src/routes/custom-providers.ts` (mounted under `/api/providers/custom`):
1. `GET    /api/providers/custom` — returns masked profiles.
2. `POST   /api/providers/custom` — creates a new profile.
3. `PATCH  /api/providers/custom/:id` — updates an existing profile.
4. `DELETE /api/providers/custom/:id` — deletes a profile.
5. `POST   /api/providers/custom/:id/probe` — tests connection and updates discovered models.

---

### Phase 5 — Anthropic-Compatible Engine

#### Tasks
1. Add `@ai-sdk/anthropic` (or native fetch to `{baseUrl}/v1/messages`).
2. `apps/server/providers/src/custom/anthropic-compatible/stream-fn.ts`:
   - Uses `createAnthropic({ baseURL: profile.baseUrl, apiKey: profile.apiKey })`.
   - `x-api-key` and `anthropic-version: 2023-06-01` headers.
3. `apps/server/providers/src/custom/anthropic-compatible/discovery.ts`:
   - `GET {baseUrl}/v1/models` probe.

---

## 4. Verification Checklist

1. **Profile Storage**:
   - Save profile with API key. Assert file exists on disk.
   - Read via API and assert key is masked (`sk-…abcd`).
2. **OpenRouter Live Test**:
   - Add OpenRouter profile with test API key.
   - Probe models via `GET /models`.
   - Send prompt `What is 2+2?` and verify streaming text response.
   - Execute `read_file` tool call and verify tool call + tool result round-trip.
3. **UI Integration**:
   - Verify `/api/providers` returns `custom:<id>`.
   - Verify model selector on mobile and desktop shows custom models.
