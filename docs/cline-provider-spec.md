# Cline Provider — Implementation Plan (v1)

## 1. Scope

Ship a Cline provider slot in the engine, **all 18 free models** for v1 (`:free` suffix). Free-tier IDs are filtered at runtime from `/v1/models` so the engine stays in sync with what Cline actually serves — adding a new free model upstream surfaces automatically. Verified end-to-end against the live API on the dates in section 10.

### In scope (v1)
- Engine wire layer (`stream-fn`, `convert-messages`, `convert-tools`, `discovery`, `auth`, `constants`, `index`)
- Provider registry entry + types
- Server-side API key login/logout/status routes
- Storage of the key at `~/.console/cline-creds.json` with `CLINE_API_KEY` env var override
- Offline tests (no LLM calls) + real-API smoke test gated by `CLINE_REAL_API=1`
- All 18 free models registered (`:free` suffix), discovered live from `/v1/models`
- Per-model context windows maintained in a small static lookup table (see 2.4) — `/v1/models` does not return a context window
- Free-tier suffix convention: `":free"`

### Out of scope (v1)
- Paid models (any non-`:free` ID is filtered out at runtime)
- Auto-fallback to the next free model on 429. Surface the error for now.
- Mobile account-management screen. API key via env var for testing; UI in a follow-up PR.
- Streaming reasoning (`delta.reasoning`). Confirmed absent on `minimax-m3:free` — if a future model exposes it, add it then.

### Non-goals
- No changes to the agent loop, tool executor, system prompt, permissions, or compaction.
- No changes to any other provider.

---

## 2. Live API behavior (verified)

Confirmed against `https://api.cline.bot/api/v1` on the dates in section 10.

### 2.1 Endpoint capabilities

| Behavior | Result |
|---|---|
| `GET /v1/models` | 396 models returned; 18 have `:free` suffix |
| Response fields per model | Only `{ id, object, created, owned_by }` — no `context_window`, no capability flags |
| Stream plain text | ✅ `delta.content` fragments, `finish_reason: "stop"`, `data: [DONE]` |
| Tool calling | ✅ `delta.tool_calls[].function.arguments` streamed as JSON string fragment, `finish_reason: "tool_calls"` |
| Image input | ✅ `image_url` data URL accepted on `minimax-m3:free` |
| `delta.reasoning` | ❌ not emitted by any tested free model |
| `cost` field | Always `0` on successful free-model calls |
| 429 on free model | ⚠️ `stream_initialization_failed`, `upstream_provider_shared_pool`, retry-after ~5s. Currently the case for `z-ai/glm-5.2:free` |

### 2.2 Free-tier filter rule (confirmed)
A model is "free" if and only if its ID ends with `:free`. No exceptions. All 18 free IDs are in scope for v1.

### 2.3 Free model catalog (all 18)

| ID | Provider | Context window |
|---|---|---|
| `inclusionai/ling-3.0-flash-fin:free` | inclusionai | 128_000 |
| `dots-studio/dots-3-note-preview:free` | dots-studio | 32_000 |
| `liquid/lfm-2.5-2.6b:free` | liquid | 32_000 |
| `nvidia/nemotron-3.5-lightning:free` | nvidia | 1_000_000 |
| `thinkingmachines/inkling-small:free` | thinkingmachines | 32_000 |
| `poolside/laguna-s-2.1:free` | poolside | 200_000 |
| `thinkingmachines/inkling:free` | thinkingmachines | 200_000 |
| `poolside/laguna-xs-2.1:free` | poolside | 200_000 |
| `cohere/north-mini-code:free` | cohere | 200_000 |
| `z-ai/glm-5.2:free` | z-ai | 200_000 |
| `nvidia/nemotron-3.5-content-safety:free` | nvidia | 200_000 |
| `nvidia/nemotron-3-ultra-550b-a55b:free` | nvidia | 1_000_000 |
| `minimax/minimax-m3:free` | minimax | 200_000 |
| `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | nvidia | 200_000 |
| `google/gemma-4-26b-a4b-it:free` | google | 200_000 |
| `google/gemma-4-31b-it:free` | google | 200_000 |
| `minimax/minimax-m2.7:free` | minimax | 200_000 |
| `nvidia/nemotron-3-super-120b-a12b:free` | nvidia | 200_000 |

### 2.4 On context windows (important)

`/v1/models` **does not return a context window field**. The values in 2.3 are best-effort defaults based on the underlying model's known capabilities (NVIDIA Nemotron 1M, Gemma 200k, Poolside 200k, etc.). They are maintained in a small static lookup table inside `cline/constants.ts`.

**This is the only hardcoded data in the v1 plan.** Reasoning for keeping it static:
- `/v1/models` returns only `{ id, object, created, owned_by }` — there's no field to read
- Inferring the window from a chat call is wasteful and slow
- The values rarely change; when one does, the lookup table is the single edit point
- The window value is only used by compaction threshold math (80% of `contextWindow`); a wrong-by-50% value just means compaction fires 50% earlier or later, not a correctness bug

If Cline ever adds a `context_window` (or similarly named) field to `/v1/models`, the discovery function should prefer it over the lookup table. The lookup becomes a fallback for unlisted models.

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
- `CLINE_FREE_MODEL_IDS` — tuple of all 18 free model IDs, populated from section 2.3. Used as the **static fallback** when the live API is unreachable. New free models added upstream surface automatically via discovery; the static list is just an offline safety net.
- `CLINE_CONTEXT_WINDOWS: Record<string, number>` — best-effort context window per free model ID. Keys must be a subset of `CLINE_FREE_MODEL_IDS`. Used by `discovery.ts` to populate `Model.contextWindow`. `CLINE_CONTEXT_WINDOW_DEFAULT = 200_000` for any ID not in the table.
- `CLINE_SUPPORTS_IMAGES: Record<string, boolean>` — per-model image-input capability. The verified default for v1 is that all 18 free models accept images (confirmed on `minimax-m3:free`). Set to `true` for all entries; flip individual entries to `false` if testing proves otherwise.
- All arrays `as const`; lookup records typed strictly.

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
  - `reasoning-delta` → `{ type: "thinking", text }` (best-effort; not emitted by any v1 free model)
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
- `getClineContextWindow(id: string): number` → returns `CLINE_CONTEXT_WINDOWS[id] ?? CLINE_CONTEXT_WINDOW_DEFAULT`
- `getClineSupportsImages(id: string): boolean` → returns `CLINE_SUPPORTS_IMAGES[id] ?? false`
- `fetchClineFreeModels(signal?)`:
  - If no credential, return static fallback
  - Else `GET ${CLINE_BASE_URL}/models` with `Authorization: Bearer <key>`, `X-Title: Console`
  - Parse `{ data: [{ id }] }`, filter to `isClineFreeModelId(id)`
  - For each surviving ID, build a `Model` object: `{ id, provider: "cline", contextWindow: getClineContextWindow(id), supportsImages: getClineSupportsImages(id) }`
  - **No allowlist filter** — any `:free` ID returned by the API is registered. Cline is the source of truth for what counts as free.
  - Sort by ID for deterministic ordering
  - On any error / non-2xx / empty result, return static fallback (the 18 IDs from `CLINE_FREE_MODEL_IDS` mapped via the same `getClineContextWindow` / `getClineSupportsImages` helpers)
- **All 18 free IDs are equally first-class.** No model is "vetted" or "unvetted" — the runtime filter is the `:free` suffix, full stop.

### 5.7 `apps/server/providers/src/cline/index.ts`
Barrel exporting: `clineStreamFn`, `fetchClineFreeModels`, `isClineFreeModelId`, `getClineContextWindow`, `getClineSupportsImages`, `CLINE_FREE_MODEL_IDS`, `CLINE_CONTEXT_WINDOWS`, `CLINE_SUPPORTS_IMAGES`, `loadClineCredential`, `saveClineCredential`, `clearClineCredential`, and the `ClineCredential` type.

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
- Add imports for `clineStreamFn`, `fetchClineFreeModels`, `getClineContextWindow`, `getClineSupportsImages`, `CLINE_FREE_MODEL_IDS`, `CLINE_CONTEXT_WINDOWS`, `CLINE_SUPPORTS_IMAGES` from the new barrel
- Add `DEFAULT_CLINE_MODELS: Model[]` derived from `CLINE_FREE_MODEL_IDS.map(id => ({ id, provider: "cline", contextWindow: getClineContextWindow(id), supportsImages: getClineSupportsImages(id) }))`. All 18 entries.
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
| 6 | Discovery returns free IDs from API | Mock fetch returns `/v1/models` with all 18 free IDs; `fetchClineFreeModels` returns all 18 (no allowlist filter) |
| 7 | Discovery falls back when no creds | `CLINE_API_KEY` unset; `fetchClineFreeModels` returns the 18-entry static fallback |
| 8 | Discovery falls back on network error | Mock fetch throws; `fetchClineFreeModels` returns the 18-entry static fallback |
| 9 | Provider registry has cline entry | `PROVIDER_CATALOG["cline"]` exists, `authMethod === "api-key"`, `models.length === 18`, all IDs end with `:free`, all have `supportsImages: true`, `contextWindow` matches `CLINE_CONTEXT_WINDOWS` for each ID |
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

- **Add new free models as they appear on Cline.** No code change required — `fetchClineFreeModels` picks them up automatically. If the new model has a non-default context window, add it to `CLINE_CONTEXT_WINDOWS` in `constants.ts`. (If a new free model comes from a known family with a known window, fill it in. If unknown, `CLINE_CONTEXT_WINDOW_DEFAULT` is fine until measured.)
- **Auto-fallback to the next free model on 429.** Currently we surface the error. If free-model flakiness becomes a complaint, add a fallback in `clineStreamFn` that catches 429s in the stream initializer and retries with the next ID from the discovered list.
- **Mobile account-management screen.** Add a Cline entry under Accounts that hits `GET /api/auth/cline/status`, shows a text input + "Save" button, calls `POST /api/auth/cline/login`, and a "Sign out" button calling `POST /api/auth/cline/logout`.
- **Streaming reasoning.** If a future free model exposes `delta.reasoning`, the existing AI SDK `reasoning-delta` mapping in `clineStreamFn` will surface it as `LLMDelta.type === "thinking"`. No code change needed; verify by testing a reasoning-capable free model once one is registered.
- **BYOK routing.** Cline's 429 error message hints at "add your own key to OpenRouter to accumulate rate limits". A future feature could let users provide their own OpenRouter key and route free models through it. Not now.
- **Drop the static `CLINE_FREE_MODEL_IDS` fallback** if/when Cline has been observed to be reliably reachable — at that point the live API is the only source of truth and the offline fallback becomes dead code.

---

## 9. Open questions

None for v1. The plan is self-contained.

## 10. Verification provenance

- Live `/v1/models` call: confirmed 396 models, 18 free (suffix `:free`).
- Live stream test on `minimax/minimax-m3:free`: text deltas, tool-call deltas, image input, `cost: 0` — all working.
- 429 test on `z-ai/glm-5.2:free`: confirmed `upstream_provider_shared_pool` rate limit (Decart upstream).
- API key used for verification was rotated by the user after testing.
