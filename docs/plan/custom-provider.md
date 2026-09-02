# Generic Wire Providers Architecture & Implementation Guide

Unifies third-party and user-defined LLM endpoints into a protocol-driven provider system.
Following the architecture proven in `oh-my-pi`, services like **OpenRouter, DeepSeek, Groq, Mistral, Together, Ollama, LM Studio, vLLM**, and arbitrary private proxies are **not** snowflake providers. They are declarative configurations running on top of a single **OpenAI-Compatible Wire Engine** (and eventually an Anthropic Messages Wire Engine).

Target layout: `apps/server/providers/src/wire/` and `apps/server/providers/src/custom/`

---

## 1. Architectural Principles

### 1.1 Wire Protocol vs Provider Instance
- A **Wire Protocol** is the transport and serialization dialect:
  - `openai-compatible`: Standard `/v1/chat/completions` + `/v1/models` SSE stream with tool-call streaming and reasoning/thinking support.
  - `anthropic-compatible`: Anthropic Messages API (`/v1/messages` with `x-api-key`).
- A **Provider Instance** is simply a declarative configuration binding to a wire protocol:
  ```ts
  interface ProviderEndpointConfig {
    id: string;               // e.g. "openrouter", "deepseek", "ollama", or "custom:<uuid>"
    label: string;            // Display name e.g. "OpenRouter", "DeepSeek", "Local Ollama"
    protocol: "openai-compatible" | "anthropic-compatible";
    baseUrl: string;          // e.g. "https://openrouter.ai/api/v1", "http://localhost:11434/v1"
    apiKey?: string;          // Stored securely, masked on client read
    headers?: Record<string, string>; // e.g. OpenRouter HTTP-Referer, X-Title
    models?: ProviderModelConfig[];   // Discovered or manually pinned models
    isBuiltinPreset?: boolean;// True for standard bundled presets, false for user custom
    createdAt: number;
    updatedAt: number;
  }

  interface ProviderModelConfig {
    id: string;               // e.g. "anthropic/claude-3.7-sonnet", "deepseek-reasoner"
    name?: string;            // Human-readable label
    contextWindow?: number;   // Default: 128_000
    supportsImages?: boolean; // Default: true
  }
  ```

### 1.2 Preset Catalog vs Custom Profiles
Both presets and custom endpoints share the exact same runtime pipeline:
- **Built-in Presets** (zero setup beyond setting an API key):
  - `openrouter`: `https://openrouter.ai/api/v1` (with `HTTP-Referer: ...` and `X-Title: ...`)
  - `deepseek`: `https://api.deepseek.com`
  - `groq`: `https://api.groq.com/openai/v1`
  - `ollama`: `http://127.0.0.1:11434/v1` (keyless by default)
  - `lm-studio`: `http://127.0.0.1:1234/v1` (keyless by default)
- **Custom Endpoints**:
  - Any URL entered by the user (self-hosted vLLM, corporate proxies, custom fine-tunes).
  - Addressed as `custom:<id>`.

---

## 2. Technical Invariants & Gotchas

1. **Zod Tool Schema Wrapping**:
   Tools passed to `streamText` must use the SDK's `tool()` helper with the Zod schema under `inputSchema:`. Never pass raw JSON Schema under `parameters:` (which triggers `asSchema(undefined)` collapsing schemas into empty objects).
2. **Streaming Protocol Parity**:
   The engine maps:
   - `text-delta` -> `AgentSessionEvent` with text chunks
   - `reasoning-delta` / `thinking` -> thinking chunks
   - `tool-input-start` / `tool-input-delta` -> progressive tool argument accumulation
3. **Keyless Local Support**:
   Endpoints running on `localhost` or `127.0.0.1` (Ollama, LM Studio) do not require `Authorization` headers when no key is set.
4. **Key Masking**:
   Any stored `apiKey` is masked on client reads (`sk-…<last4>`). Plain keys are only accessible in the server runtime stream factory.
5. **Universal Discovery**:
   A single discovery function `fetchModelsForEndpoint(endpoint)` calls `GET {baseUrl}/models` with Bearer auth, normalizes models, and falls back gracefully to manual model definitions when `/models` is disabled or blocked.

---

## 3. Phased Implementation Plan

### Phase 1: Shared OpenAI-Compatible Wire Engine
**Goal**: A rock-solid, reusable stream engine that powers any OpenAI-compatible endpoint.

- `apps/server/providers/src/wire/openai-compatible/stream-fn.ts`:
  - `createOpenAICompatibleStreamFn(options: { baseUrl, apiKey?, headers? }): StreamFn`
  - Built on `@ai-sdk/openai-compatible` and `streamText`.
  - Emits `text`, `thinking`, and `toolCall` deltas.
- `apps/server/providers/src/wire/openai-compatible/convert-tools.ts`:
  - Shared Zod tool schema wrapper.
- `apps/server/providers/src/wire/openai-compatible/convert-messages.ts`:
  - Shared message format converter.
- `apps/server/providers/src/wire/openai-compatible/discovery.ts`:
  - Generic `fetchOpenAICompatibleModels(baseUrl, apiKey?, headers?)`.
- **Verification**: `apps/server/tests/wire-openai-compatible.test.ts` (mock server verifying streaming text, tool calling, and thinking chunks).

---

### Phase 2: Preset Definitions & Endpoint Store
**Goal**: Store credentials for presets (OpenRouter, DeepSeek, Groq, Ollama) and user custom endpoints.

- `apps/server/providers/src/custom/presets.ts`:
  - Declarative preset definitions:
    - `openrouter`: `https://openrouter.ai/api/v1`, headers, default model IDs.
    - `deepseek`: `https://api.deepseek.com`, models `deepseek-chat`, `deepseek-reasoner`.
    - `groq`: `https://api.groq.com/openai/v1`, fast inference models.
    - `ollama`: `http://127.0.0.1:11434/v1`, keyless local defaults.
- `apps/server/providers/src/custom/endpoint-store.ts`:
  - Persisted under `<storageDir>/endpoints.json` (or `custom-providers.json`).
  - CRUD operations: `listEndpoints()`, `getEndpoint(id)`, `saveEndpoint(config)`, `deleteEndpoint(id)`.
  - `maskEndpoint(endpoint)` for API outputs.
- **Verification**: `apps/server/tests/endpoint-store.test.ts` (CRUD, key masking, persistence across re-instantiations).

---

### Phase 3: Registry Integration & Dynamic Model Injection
**Goal**: Expose both presets and custom endpoints to the client without modifying front-end chat screens.

- `apps/server/agent/src/commands/provider-registry.ts`:
  - In `getProvider(id)`:
    - If `id` matches a preset (e.g. `openrouter`, `deepseek`, `groq`, `ollama`) or starts with `custom:`, resolve endpoint from store/presets and instantiate `createOpenAICompatibleStreamFn(endpoint)`.
  - In `listProviders()`:
    - Append active presets (those with configured keys or keyless local engines) and custom profiles.
    - Expose their models with correct context windows.
- `apps/server/api/src/routes/endpoints.ts`:
  - `GET    /api/endpoints` (list all configured presets & custom endpoints, masked)
  - `POST   /api/endpoints` (save/update endpoint configuration)
  - `DELETE /api/endpoints/:id` (delete custom endpoint)
  - `POST   /api/endpoints/:id/probe` (probe connection and fetch models)
- **Verification**: `apps/server/tests/provider-registry-wire.test.ts` (resolving providers, listing models in API).

---

### Phase 4: Anthropic-Compatible Wire Engine
**Goal**: Support Anthropic Messages API format for compatible proxies.

- `apps/server/providers/src/wire/anthropic-compatible/stream-fn.ts`:
  - `createAnthropicCompatibleStreamFn(options: { baseUrl, apiKey })`.
  - Maps top-level system, `x-api-key`, and tool calls.
- `apps/server/providers/src/wire/anthropic-compatible/discovery.ts`:
  - Probes `GET {baseUrl}/v1/models`.

---

## 4. Verification & Testing Matrix

| Test Case | Method | Expected Outcome |
| :--- | :--- | :--- |
| **OpenAI Wire Unit** | `bun tests/wire-openai-compatible.test.ts` | Streams text, tool calls, and thinking with mocked HTTP |
| **Local Ollama** | Keyless local test (`http://127.0.0.1:11434/v1`) | Connects and executes without Authorization header |
| **OpenRouter Preset** | Live or mock with Bearer token | Passes `HTTP-Referer` and executes chat + tools |
| **Client Model Picker** | `GET /api/providers` | Custom and active preset endpoints appear with models |
| **Key Masking** | API query verification | Never leaks full API key in logs or JSON responses |
