# Cline Provider — Implementation Plan (v1)

## 1. Scope

Ship a Cline provider slot in the engine, **single free model** for v1: `minimax/minimax-m3:free`. Verified end-to-end against the live API on the dates in section 10.

### In scope (v1)
- Engine wire layer (`stream-fn`, `convert-messages`, `convert-tools`, `discovery`, `auth`, `constants`, `index`)
- Provider registry entry + types
- Server-side API key login/logout/status routes
- Storage of the key at `~/.console/cline-creds.json` with `CLINE_API_KEY` env var override
- Offline tests (no LLM calls) + real-API smoke test gated by `CLINE_REAL_API=1`
- Static fallback model list = `["minimax/minimax-m3:free"]` (1 entry)
- Free-tier suffix convention: `":free"`

### Out of scope (v1)
- The other 17 free models. Add later by extending `CLINE_FREE_MODEL_IDS` — no code change.
- Auto-fallback to the next free model on 429. Surface the error for now.
- Mobile account-management screen. API key via env var for testing; UI in a follow-up PR.
- Streaming reasoning (`delta.reasoning`). Confirmed absent on `minimax-m3:free` — if a future model exposes it, add it then.

### Non-goals
- No changes to the agent loop, tool executor, system prompt, permissions, or compaction.
- No changes to any other provider.

---

## 2. Live API behavior (verified)

Confirmed against `https://api.cline.bot/api/v1` on the dates in section 10.

| Behavior | Result |
|---|---|
| `GET /v1/models` | 396 models returned; 18 have `:free` suffix |
| Stream plain text | ✅ `delta.content` fragments, `finish_reason: "stop"`, `data: [DONE]` |
| Tool calling | ✅ `delta.tool_calls[].function.arguments` streamed as JSON string fragment, `finish_reason: "tool_calls"` |
| Image input | ✅ `image_url` data URL accepted on `minimax-m3:free` |
| `delta.reasoning` | ❌ not emitted by any tested free model |
| `cost` field | Always `0` on successful free-model calls |
| 429 on free model | ⚠️ `stream_initialization_failed`, `upstream_provider_shared_pool`, retry-after ~5s. Currently the case for `z-ai/glm-5.2:free` |

### Free-tier filter rule (confirmed)
A model is "free" if and only if its ID ends with `:free`. No exceptions. v1 ships only `minimax/minimax-m3:free`.

---

## 3. Endpoint contract

Endpoint: `POST https://api.cline.bot/api/v1/chat/completions`

Required headers:
- `Authorization: Bearer <CLINE_API_KEY>`
- `X-Title: Console`
- `Content-Type: application/json`

Request body: standard OpenAI Chat Completions. Pass `stream: true`.

Response: SSE with `data: {...}\n\n` chunks, terminated by `data: [DONE]\n\n`. Schema fields used: `delta.content` (string), `delta.tool_calls[]` (object with `id`, `type: "function"`, `function.name`, `function.arguments`).

The AI SDK's `createOpenAICompatible` already handles all of this. We mirror the OpenCode pattern.

---

## 4. File changes

### 4.1 New files

| File | Purpose |
|---|---|
| `apps/server/providers/src/cline/constants.ts` | Base URL, model ID list, default context window |
| `apps/server/providers/src/cline/auth.ts` | `loadClineCredential` / `saveClineCredential` / `clearClineCredential` |
| `apps/server/providers/src/cline/stream-fn.ts` | `clineStreamFn: StreamFn` |
| `apps/server/providers/src/cline/convert-messages.ts` | Re-export of `convertOpencodeMessages` (same wire format) |
| `apps/server/providers/src/cline/convert-tools.ts` | Re-export of `convertOpencodeTools` (same wire format) |
| `apps/server/providers/src/cline/discovery.ts` | `fetchClineFreeModels` + `isClineFreeModelId` |
| `apps/server/providers/src/cline/index.ts` | Barrel re-exports |
| `apps/server/api/src/routes/cline.ts` | `GET /cline/status`, `POST /cline/login`, `POST /cline/logout` |
| `apps/server/tests/cline.test.ts` | Offline + real-API tests |

### 4.2 Edited files

| File | Change |
|---|---|
| `packages/types/src/model.ts` | Add `"cline"` to `ProviderId`; add `"api-key"` to `authMethod` |
| `apps/server/agent/src/commands/provider-registry.ts` | Import Cline barrel, add `DEFAULT_CLINE_MODELS`, add `cline` to `PROVIDER_CATALOG`, extend `fetchModelsForProvider` switch + static-fallback ternary |
| `apps/server/providers/src/index.ts` | Re-export Cline barrel |
| `apps/server/api/src/routes/providers.ts` | Whitelist `"cline"` in provider validation |
| `apps/server/api/src/routes/auth.ts` | Mount the new Cline auth sub-router |

---

## 5. Per-file design notes

### 5.1 `apps/server/providers/src/cline/constants.ts`
- `CLINE_BASE_URL = "https://api.cline.bot/api/v1"`
- `CLINE_FREE_MODEL_IDS = ["minimax/minimax-m3:free"]` — single entry for v1
- `CLINE_CONTEXT_WINDOW_DEFAULT = 200_000`
- All `as const`. Free IDs are vetted explicitly; the 17 other free models are deliberately excluded.

### 5.2 `apps/server/providers/src/cline/auth.ts`
- `interface ClineCredential { apiKey: string }`
- Storage path: `~/.console/cline-creds.json`, overridable via `CLINE_CREDENTIALS_PATH`
- `loadClineCredential()` lookup precedence: `CLINE_API_KEY` env var → stored file → `null`
- `saveClineCredential(cred)` writes JSON, creates parent dir
- `clearClineCredential()` deletes the file; missing-file is silent
- Mirrors the simplicity of `codex/oauth.ts` but for a single key field instead of OAuth tokens

### 5.3 `apps/server/providers/src/cline/stream-fn.ts`
- Factory: `clineStreamFn: StreamFn` (matches `opencodeStreamFn` shape)
- On each call: read credential via `loadClineCredential()`. If null, throw with a clear "not configured" message.
- Build a fresh `createOpenAICompatible({ name, baseURL, apiKey, headers: { "X-Title": "Console" } })` per call. The key is per-user, not per-process.
- Call `streamText` with `model: cline.chatModel(model.id)`, `system`, `messages = convertOpencodeMessages(messages)`, `tools = convertOpencodeTools(tools)` (only if non-empty), `abortSignal: signal`, `onError` to capture.
- `for await (const part of result.fullStream)`:
  - `text-delta` → `{ type: "text", text }`
  - `reasoning-delta` → `{ type: "thinking", text }` (best-effort; not emitted by v1 model)
  - `tool-input-start` → `{ type: "toolCall", id, name, argumentsJson: "" }`
  - `tool-input-delta` → `{ type: "toolCall", id, name: "", argumentsJson: delta }`
  - `error` → throw
  - `tool-call` → ignore (already accumulated)
- After the loop, re-throw any captured `streamError`

### 5.4 `apps/server/providers/src/cline/convert-messages.ts`
- One-line re-export of `convertOpencodeMessages` from the OpenCode provider
- Same wire format, no transformation needed

### 5.5 `apps/server/providers/src/cline/convert-tools.ts`
- One-line re-export of `convertOpencodeTools` from the OpenCode provider

### 5.6 `apps/server/providers/src/cline/discovery.ts`
- `isClineFreeModelId(id: string): boolean` → `id.endsWith(":free")`
- `fetchClineFreeModels(signal?)`: if no credential, return static fallback. Else `GET ${CLINE_BASE_URL}/models` with `Authorization: Bearer <key>`, `X-Title: Console`, parse `{ data: [{ id }] }`, filter `isClineFreeModelId`, then **filter again against `CLINE_FREE_MODEL_IDS` allowlist** (this is the safety net: 17 free IDs are filtered out at runtime even if they appear in the upstream response), map to `Model[]` with `contextWindow: CLINE_CONTEXT_WINDOW_DEFAULT`, `supportsImages: true`. On any error/non-2xx, return static fallback.
- `fallbackClineModels()`: returns `CLINE_FREE_MODEL_IDS` mapped to `Model[]`

### 5.7 `apps/server/providers/src/cline/index.ts`
Barrel exporting: `clineStreamFn`, `fetchClineFreeModels`, `isClineFreeModelId`, `CLINE_FREE_MODEL_IDS`, `loadClineCredential`, `saveClineCredential`, `clearClineCredential`, and the `ClineCredential` type.

### 5.8 `apps/server/api/src/routes/cline.ts`
Three endpoints, mounted under `/api/auth/cline/*`:
- `GET /cline/status` → `{ success: true, data: { loggedIn: cred !== null } }`
- `POST /cline/login` — body `{ apiKey: string }`. Validates the key by probing `GET /v1/models` with it. If non-2xx, return 400. If 2xx, `saveClineCredential({ apiKey })`, return `{ success: true, data: { loggedIn: true } }`. The probe is the key correctness check.
- `POST /cline/logout` — `clearClineCredential()`, return `{ success: true, data: { loggedIn: false } }`

### 5.9 `packages/types/src/model.ts`
Two string-union changes:
- `ProviderId`: add `"cline"`
- `ProviderCatalogEntry.authMethod`: add `"api-key"` as a fourth value

### 5.10 `apps/server/agent/src/commands/provider-registry.ts`
Three edits:
- Add imports for `clineStreamFn`, `fetchClineFreeModels`, `CLINE_FREE_MODEL_IDS` from the new barrel
- Add `DEFAULT_CLINE_MODELS: Model[]` derived from `CLINE_FREE_MODEL_IDS.map(id => ({ id, provider: "cline", contextWindow: 200_000, supportsImages: true }))`
- Add `cline` to `PROVIDER_CATALOG` with `authMethod: "api-key"`, `models: DEFAULT_CLINE_MODELS`, `getStreamFn: () => clineStreamFn`
- Extend the `fetchModelsForProvider` chain: `else if (providerName === "cline") discovered = await fetchClineFreeModels(signal)`
- Extend the static-fallback ternary in the same function to include `providerName === "cline" ? DEFAULT_CLINE_MODELS : ...`

### 5.11 `apps/server/providers/src/index.ts`
Add a Cline section re-exporting the barrel's public surface (stream fn, discovery, constants, auth helpers) and the `ClineCredential` type.

### 5.12 `apps/server/api/src/routes/providers.ts`
Whitelist `"cline"` in the existing `if (providerId !== "..." && ...)` validation block. One line added.

### 5.13 `apps/server/api/src/routes/auth.ts`
Import `clineAuthRoutes` from `./cline.js` and mount it via `api.route("/auth", clineAuthRoutes)`. Result: paths become `/api/auth/cline/status`, `/api/auth/cline/login`, `/api/auth/cline/logout`.

---

## 6. Test plan — `apps/server/tests/cline.test.ts`

Mirrors `tests/opencode.test.ts`. No LLM calls in offline cases; one real-API test gated by env.

| # | Test | Asserts |
|---|---|---|
| 1 | Message converter round-trip | `convertOpencodeMessages` re-export produces correct AI SDK `ModelMessage[]` for user + assistant-with-toolcall + toolResult history |
| 2 | Tool converter round-trip | `convertOpencodeTools` re-export produces a valid `ToolSet` with Zod `inputSchema` preserved |
| 3 | Stream fn without credentials throws | `clineStreamFn` with no `CLINE_API_KEY` and no stored credential throws mentioning "not configured" |
| 4 | Stream fn with credentials yields deltas | Set `CLINE_API_KEY` in `process.env`; mock the AI SDK; assert text + toolCall deltas |
| 5 | Free-tier filter | `isClineFreeModelId("minimax/minimax-m3:free")` → true; `isClineFreeModelId("z-ai/glm-5.2")` → false |
| 6 | Discovery returns vetted IDs | Mock fetch returns `/v1/models` with all 18 free IDs; `fetchClineFreeModels` returns only `minimax/minimax-m3:free` (allowlist filter) |
| 7 | Discovery falls back when no creds | `CLINE_API_KEY` unset; `fetchClineFreeModels` returns the static fallback |
| 8 | Discovery falls back on network error | Mock fetch throws; `fetchClineFreeModels` returns the static fallback |
| 9 | Provider registry has cline entry | `PROVIDER_CATALOG["cline"]` exists, `authMethod === "api-key"`, `models.length === 1`, `models[0].id === "minimax/minimax-m3:free"`, `supportsImages === true` |
| 10 | ProviderId union includes "cline" | Type-level check via a const assignment that wouldn't compile if "cline" weren't a valid literal |
| 11 | `cline.login` rejects bad keys | Mock fetch returns 401; route returns 400 |
| 12 | `cline.login` saves good keys | Mock fetch returns 200; `saveClineCredential` called with `{ apiKey }` |
| 13 | `cline.status` reflects stored cred | With a stub creds file in a temp dir, status returns `loggedIn: true` |
| 14 | **Real API** (gated by `CLINE_REAL_API=1`) | Live `POST /v1/chat/completions` with a tool; assert tool-call deltas assemble into valid args |

Tests 1, 2, 5, 6, 7, 8, 9, 10, 11, 12, 13 are pure offline. Tests 3, 4 inject a credential via `process.env.CLINE_API_KEY` then `delete process.env.CLINE_API_KEY` in cleanup.

Run command (per AGENTS.md):
- `cd apps/server && bun tests/cline.test.ts`

---

## 7. Rollout

| # | Step | Risk | Notes |
|---|---|---|---|
| 1 | Add `"cline"` + `"api-key"` to types | None | Compiles or doesn't |
| 2 | Add `cline/` provider folder (constants, auth, convert, discovery) | Low | Pure functions, easy to test |
| 3 | Add `clineStreamFn` | Medium | First time the wire layer runs against Cline's auth |
| 4 | Wire into `provider-registry.ts` | Low | Mechanical |
| 5 | Add `cline.test.ts` | None | Verification step |
| 6 | Add `routes/cline.ts` and mount in `auth.ts` | Low | Mirrors existing route patterns |
| 7 | Whitelist `"cline"` in `routes/providers.ts` | None | One line |
| 8 | Run `bun tests/cline.test.ts` | Verification | All 13 offline cases pass |
| 9 | (Optional) Run with `CLINE_REAL_API=1` against a real key | Verification | Confirms live API works |
| 10 | Commit + push | Done | One PR, one-line commit message |

Ship as **one PR**. The diff is small enough that splitting "engine" from "API" doesn't pay off.

---

## 8. Future work (explicitly NOT in v1)

- **Add the other 17 free models.** One-line change to `CLINE_FREE_MODEL_IDS`. No code change needed. The `fetchClineFreeModels` allowlist filter ensures unvetted IDs are filtered out at runtime, so adding a new ID is the explicit "I trust this model" gesture.
- **Auto-fallback to the next free model on 429.** Currently we surface the error. If free-model flakiness becomes a complaint, add a fallback in `clineStreamFn` that catches 429s in the stream initializer and retries with the next ID from `CLINE_FREE_MODEL_IDS`.
- **Mobile account-management screen.** Add a Cline entry under Accounts that hits `GET /api/auth/cline/status`, shows a text input + "Save" button, calls `POST /api/auth/cline/login`, and a "Sign out" button calling `POST /api/auth/cline/logout`.
- **Streaming reasoning.** If a future free model exposes `delta.reasoning`, the existing AI SDK `reasoning-delta` mapping in `clineStreamFn` will surface it as `LLMDelta.type === "thinking"`. No code change needed; verify by testing a reasoning-capable free model once one is registered.
- **BYOK routing.** Cline's 429 error message hints at "add your own key to OpenRouter to accumulate rate limits". A future feature could let users provide their own OpenRouter key and route free models through it. Not now.

---

## 9. Open questions

None for v1. The plan is self-contained.

## 10. Verification provenance

- Live `/v1/models` call: confirmed 396 models, 18 free (suffix `:free`).
- Live stream test on `minimax/minimax-m3:free`: text deltas, tool-call deltas, image input, `cost: 0` — all working.
- 429 test on `z-ai/glm-5.2:free`: confirmed `upstream_provider_shared_pool` rate limit (Decart upstream).
- API key used for verification was rotated by the user after testing.
