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

```
POST https://api.cline.bot/api/v1/chat/completions
Authorization: Bearer <CLINE_API_KEY>
X-Title: Console
Content-Type: application/json
```

Request body: standard OpenAI Chat Completions. We pass `stream: true`.

Response: SSE with `data: {...}\n\n` chunks, terminated by `data: [DONE]\n\n`. Schema: `delta.content` (string), `delta.tool_calls[]` (object with `id`, `type: "function"`, `function.name`, `function.arguments`).

The AI SDK's `createOpenAICompatible` already handles all of this. We mirror the OpenCode pattern.

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
| `apps/server/agent/src/commands/provider-registry.ts` | Import Cline, add `DEFAULT_CLINE_MODELS`, add `cline` to `PROVIDER_CATALOG`, extend `fetchModelsForProvider` switch + static-fallback ternary |
| `apps/server/providers/src/index.ts` | Re-export Cline barrel |
| `apps/server/api/src/routes/providers.ts` | Whitelist `"cline"` in provider validation |
| `apps/server/api/src/routes/auth.ts` | Mount the new Cline auth sub-router |

---

## 5. File contents (sketch)

### 5.1 `apps/server/providers/src/cline/constants.ts`

```ts
/**
 * Cline provider — OpenAI-compatible chat completions endpoint.
 * https://docs.cline.bot/api/overview
 *
 * Auth: Bearer token in Authorization header. No OAuth.
 *   Get key: app.cline.bot > Settings > API Keys.
 *   Env var: CLINE_API_KEY overrides stored credential.
 *
 * Free-tier filter: model IDs ending in ":free".
 * v1 ships only "minimax/minimax-m3:free".
 */
export const CLINE_BASE_URL = "https://api.cline.bot/api/v1";

/** Free Cline model IDs registered with the engine. v1 ships one. */
export const CLINE_FREE_MODEL_IDS = [
  "minimax/minimax-m3:free",
] as const;

/** Default context window when /v1/models doesn't tell us otherwise. */
export const CLINE_CONTEXT_WINDOW_DEFAULT = 200_000;
```

### 5.2 `apps/server/providers/src/cline/auth.ts`

```ts
/**
 * Cline API key storage. No OAuth — single static key.
 * Stored at ~/.console/cline-creds.json
 * Overridable via CLINE_CREDENTIALS_PATH env var.
 *
 * Lookup precedence (first non-empty wins):
 *   1. CLINE_API_KEY env var
 *   2. ~/.console/cline-creds.json
 *   3. null
 */
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";

export interface ClineCredential {
  apiKey: string;
}

function credentialFilePath(): string {
  return (
    process.env.CLINE_CREDENTIALS_PATH ??
    path.join(os.homedir(), ".console", "cline-creds.json")
  );
}

export async function loadClineCredential(): Promise<ClineCredential | null> {
  const envKey = process.env.CLINE_API_KEY?.trim();
  if (envKey) return { apiKey: envKey };

  try {
    const raw = await fs.readFile(credentialFilePath(), "utf-8");
    const parsed = JSON.parse(raw) as Partial<ClineCredential>;
    if (typeof parsed.apiKey === "string" && parsed.apiKey.length > 0) {
      return { apiKey: parsed.apiKey };
    }
    return null;
  } catch {
    return null;
  }
}

export async function saveClineCredential(cred: ClineCredential): Promise<void> {
  const filePath = credentialFilePath();
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(cred, null, 2), "utf-8");
}

export async function clearClineCredential(): Promise<void> {
  try {
    await fs.unlink(credentialFilePath());
  } catch {
    // Already gone
  }
}
```

### 5.3 `apps/server/providers/src/cline/stream-fn.ts`

```ts
/**
 * Cline StreamFn — OpenAI-compatible /v1/chat/completions via the AI SDK.
 * Same wire format as OpenCode Zen. Auth: Bearer CLINE_API_KEY.
 *
 * The key is read per-call (not at module load) so the user can add it
 * mid-session. The AI SDK client is built once per call.
 */
import { streamText } from "ai";
import { createOpenAICompatible } from "@ai-sdk/openai-compatible";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";
import { CLINE_BASE_URL } from "./constants.js";
import { convertOpencodeMessages } from "@/providers/src/opencode/convert-messages.js";
import { convertOpencodeTools } from "@/providers/src/opencode/convert-tools.js";
import { loadClineCredential } from "./auth.js";

export const clineStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
}) {
  const cred = await loadClineCredential();
  if (!cred) {
    throw new Error(
      "Cline is not configured. Set CLINE_API_KEY or add a key via POST /api/auth/cline/login.",
    );
  }

  const cline = createOpenAICompatible({
    name: "cline",
    baseURL: CLINE_BASE_URL,
    apiKey: cred.apiKey,
    headers: { "X-Title": "Console" },
  });

  const convertedMessages = convertOpencodeMessages(messages);
  const convertedTools = convertOpencodeTools(tools);

  let streamError: unknown = null;

  const result = streamText({
    model: cline.chatModel(model.id),
    system: systemPrompt,
    messages: convertedMessages,
    ...(Object.keys(convertedTools).length > 0 ? { tools: convertedTools } : {}),
    abortSignal: signal,
    onError({ error }) {
      streamError = error;
    },
  });

  for await (const part of result.fullStream) {
    if (part.type === "error") {
      throw (part as any).error ?? new Error("Cline stream error");
    }
    if (part.type === "text-delta") {
      yield { type: "text", text: part.text };
    } else if (part.type === "reasoning-delta") {
      yield { type: "thinking", text: part.text };
    } else if (part.type === "tool-input-start") {
      yield { type: "toolCall", id: part.id, name: part.toolName, argumentsJson: "" };
    } else if (part.type === "tool-input-delta") {
      yield { type: "toolCall", id: part.id, name: "", argumentsJson: part.delta };
    }
    // "tool-call" intentionally ignored: see opencode/stream-fn.ts comment.
  }

  if (streamError) {
    throw streamError;
  }
};
```

### 5.4 `apps/server/providers/src/cline/convert-messages.ts`

```ts
// Identical wire format to OpenCode Zen. Re-export to keep cline/ self-contained.
export { convertOpencodeMessages } from "@/providers/src/opencode/convert-messages.js";
```

### 5.5 `apps/server/providers/src/cline/convert-tools.ts`

```ts
export { convertOpencodeTools } from "@/providers/src/opencode/convert-tools.js";
```

### 5.6 `apps/server/providers/src/cline/discovery.ts`

```ts
/**
 * Cline model discovery.
 * GET /v1/models, filter to free-tier ids (suffix ":free").
 * Falls back to the static CLINE_FREE_MODEL_IDS on network/parse errors.
 *
 * v1 policy: surface only IDs that are registered in CLINE_FREE_MODEL_IDS.
 * The other 17 free IDs are filtered out at runtime so the model picker
 * always matches the static catalog.
 */
import type { Model } from "@console/types";
import {
  CLINE_BASE_URL,
  CLINE_CONTEXT_WINDOW_DEFAULT,
  CLINE_FREE_MODEL_IDS,
} from "./constants.js";
import { loadClineCredential } from "./auth.js";

interface ClineModelsResponse {
  data?: Array<{ id: string }>;
}

export function isClineFreeModelId(id: string): boolean {
  return id.endsWith(":free");
}

export async function fetchClineFreeModels(
  signal?: AbortSignal,
): Promise<Model[]> {
  const cred = await loadClineCredential();
  if (!cred) return fallbackClineModels();

  try {
    const response = await fetch(`${CLINE_BASE_URL}/models`, {
      method: "GET",
      headers: {
        Authorization: `Bearer ${cred.apiKey}`,
        Accept: "application/json",
        "X-Title": "Console",
      },
      signal,
    });
    if (!response.ok) return fallbackClineModels();

    const payload = (await response.json()) as ClineModelsResponse;
    const ids = (payload.data ?? []).map((m) => m.id).filter(isClineFreeModelId);
    if (ids.length === 0) return fallbackClineModels();

    const allowed = new Set<string>(CLINE_FREE_MODEL_IDS);
    const vetted = ids.filter((id) => allowed.has(id));
    if (vetted.length === 0) return fallbackClineModels();

    return vetted.map((id) => ({
      id,
      provider: "cline",
      contextWindow: CLINE_CONTEXT_WINDOW_DEFAULT,
      supportsImages: true, // verified on minimax-m3:free
    }));
  } catch {
    return fallbackClineModels();
  }
}

function fallbackClineModels(): Model[] {
  return CLINE_FREE_MODEL_IDS.map((id) => ({
    id,
    provider: "cline",
    contextWindow: CLINE_CONTEXT_WINDOW_DEFAULT,
    supportsImages: true,
  }));
}
```

### 5.7 `apps/server/providers/src/cline/index.ts`

```ts
export { clineStreamFn } from "./stream-fn.js";
export { fetchClineFreeModels, isClineFreeModelId } from "./discovery.js";
export { CLINE_FREE_MODEL_IDS } from "./constants.js";
export {
  loadClineCredential,
  saveClineCredential,
  clearClineCredential,
} from "./auth.js";
export type { ClineCredential } from "./auth.js";
```

### 5.8 `apps/server/api/src/routes/cline.ts`

```ts
/**
 * Cline auth routes (/api/auth/cline/*).
 *   GET  /cline/status   → { loggedIn: boolean }
 *   POST /cline/login    → body { apiKey: string } → saves after a live /v1/models probe
 *   POST /cline/logout   → clears the stored credential
 */
import { Hono } from "hono";
import {
  CLINE_BASE_URL,
  clearClineCredential,
  loadClineCredential,
  saveClineCredential,
} from "@/providers/src/cline/index.js";

export const clineAuthRoutes = new Hono();

clineAuthRoutes.get("/cline/status", async (c) => {
  const cred = await loadClineCredential();
  return c.json({ success: true, data: { loggedIn: cred !== null } });
});

clineAuthRoutes.post("/cline/login", async (c) => {
  const body = (await c.req.json().catch(() => null)) as { apiKey?: string } | null;
  const apiKey = body?.apiKey?.trim();
  if (!apiKey) {
    return c.json({ success: false, error: "Missing apiKey." }, 400);
  }

  // Probe /v1/models with the new key before saving. Catches typos and
  // revoked keys at login time, not at first chat.
  const probe = await fetch(`${CLINE_BASE_URL}/models`, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${apiKey}`,
      Accept: "application/json",
      "X-Title": "Console",
    },
  }).catch((err) => ({ ok: false, status: 0, _err: err } as const));

  if (!("ok" in probe) || !probe.ok) {
    return c.json(
      { success: false, error: `Cline rejected the API key (HTTP ${probe.status}).` },
      400,
    );
  }

  await saveClineCredential({ apiKey });
  return c.json({ success: true, data: { loggedIn: true } });
});

clineAuthRoutes.post("/cline/logout", async (c) => {
  await clearClineCredential();
  return c.json({ success: true, data: { loggedIn: false } });
});
```

### 5.9 `packages/types/src/model.ts`

```ts
// Add "cline" to the union
export type ProviderId = "gemini" | "antigravity" | "opencode" | "codex" | "cline";

// Add "api-key" to the auth method union
export interface ProviderCatalogEntry {
  // ... existing fields ...
  authMethod: "oauth" | "device-code" | "none" | "api-key";
}
```

### 5.10 `apps/server/agent/src/commands/provider-registry.ts`

Three additions:

```ts
// imports
import {
  clineStreamFn,
  fetchClineFreeModels,
  CLINE_FREE_MODEL_IDS,
} from "@/providers/src/cline/index.js";

// static defaults
export const DEFAULT_CLINE_MODELS: Model[] = CLINE_FREE_MODEL_IDS.map((id) => ({
  id,
  provider: "cline" as const,
  contextWindow: 200_000,
  supportsImages: true,
}));

// registry entry
cline: {
  name: "cline",
  displayName: "Cline",
  description: "Free models via the Cline OpenAI-compatible gateway",
  authMethod: "api-key",
  models: DEFAULT_CLINE_MODELS,
  getStreamFn: () => clineStreamFn,
},

// fetchModelsForProvider branch
} else if (providerName === "cline") {
  discovered = await fetchClineFreeModels(signal);
} else { ... }
```

Also add `cline` to the static-fallback ternary in the same function:

```ts
const staticFallback =
  providerName === "gemini"
    ? DEFAULT_GEMINI_MODELS
    : providerName === "opencode"
      ? DEFAULT_OPENCODE_MODELS
      : providerName === "codex"
        ? DEFAULT_CODEX_MODELS
        : providerName === "cline"
          ? DEFAULT_CLINE_MODELS
          : DEFAULT_ANTIGRAVITY_MODELS;
```

### 5.11 `apps/server/providers/src/index.ts`

```ts
/** Cline provider — OpenAI-compatible, free tier, Bearer auth */
export {
  clineStreamFn,
  fetchClineFreeModels,
  CLINE_FREE_MODEL_IDS,
  loadClineCredential,
  saveClineCredential,
  clearClineCredential,
} from "./cline/index.js";
export type { ClineCredential } from "./cline/index.js";
```

### 5.12 `apps/server/api/src/routes/providers.ts`

```ts
if (
  providerId !== "gemini" &&
  providerId !== "antigravity" &&
  providerId !== "opencode" &&
  providerId !== "codex" &&
  providerId !== "cline"        // ← add
) {
  return c.json({ success: false, error: `Invalid provider '${providerId}'.` }, 400);
}
```

### 5.13 `apps/server/api/src/routes/auth.ts`

```ts
import { clineAuthRoutes } from "./cline.js";

// in the existing authRoutes mount:
api.route("/auth", clineAuthRoutes);
```

(Mount under `/auth` so the full paths are `/api/auth/cline/status`, `/api/auth/cline/login`, `/api/auth/cline/logout`.)

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

Tests 1, 2, 5, 6, 7, 8, 9, 10, 11, 12, 13 are pure offline. Test 3, 4 inject a credential via `process.env.CLINE_API_KEY` then `delete process.env.CLINE_API_KEY` in cleanup.

Run command (per AGENTS.md):
```bash
cd apps/server && bun tests/cline.test.ts
```

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
