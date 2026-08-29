/**
 * Unit Tests for the Cline provider.
 * Operates offline — uses mock fetch responses (0 LLM credits consumed).
 *
 * Covers:
 *  1. isClineFreeModelId → :free suffix filter
 *  2. getClineContextWindow / getClineSupportsImages lookups
 *  3. fetchClineFreeModels → live-API discovery with :free filtering
 *  4. fetchClineFreeModels → static fallback when no credentials
 *  5. fetchClineFreeModels → static fallback on network error
 *  6. fetchClineFreeModels → static fallback on non-2xx response
 *  7. loadClineCredential / saveClineCredential / clearClineCredential
 *  8. provider-registry catalog entry for "cline" (all 18 free models)
 *  9. Cline auth route — login rejects bad keys, accepts good keys, status reflects state
 */
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import {
  listProviders,
  listModelsForProvider,
} from "@/agent/src/commands/provider-registry.js";
import {
  CLINE_FREE_MODEL_IDS,
  CLINE_CONTEXT_WINDOWS,
  CLINE_CONTEXT_WINDOW_DEFAULT,
  getClineContextWindow,
  getClineSupportsImages,
  isClineFreeModelId,
  fetchClineFreeModels,
  loadClineCredential,
  saveClineCredential,
  clearClineCredential,
} from "@/providers/src/cline/index.js";
import { CLINE_BASE_URL } from "@/providers/src/cline/constants.js";

console.log("Running Cline Provider tests...");

// 1. isClineFreeModelId
{
  assert.equal(isClineFreeModelId("minimax/minimax-m3:free"), true);
  assert.equal(isClineFreeModelId("z-ai/glm-5.2:free"), true);
  assert.equal(isClineFreeModelId("anthropic/claude-sonnet-4-6"), false);
  assert.equal(isClineFreeModelId("z-ai/glm-5.2"), false);
  assert.equal(isClineFreeModelId(""), false);
  console.log("  ✅ isClineFreeModelId → :free suffix");
}

// 2. getClineContextWindow / getClineSupportsImages
{
  assert.equal(getClineContextWindow("minimax/minimax-m3:free"), 200_000);
  assert.equal(getClineContextWindow("nvidia/nemotron-3.5-lightning:free"), 1_000_000);
  assert.equal(getClineContextWindow("nvidia/nemotron-3-ultra-550b-a55b:free"), 1_000_000);
  assert.equal(getClineContextWindow("unknown-model:free"), CLINE_CONTEXT_WINDOW_DEFAULT);
  assert.equal(getClineContextWindow("anthropic/claude-sonnet-4-6"), CLINE_CONTEXT_WINDOW_DEFAULT);

  assert.equal(getClineSupportsImages("minimax/minimax-m3:free"), true);
  assert.equal(getClineSupportsImages("nvidia/nemotron-3.5-lightning:free"), false);
  assert.equal(getClineSupportsImages("unknown-model:free"), false);

  for (const id of CLINE_FREE_MODEL_IDS) {
    assert.ok(CLINE_CONTEXT_WINDOWS[id] !== undefined, `missing context window for ${id}`);
  }
  console.log("  ✅ getClineContextWindow / getClineSupportsImages lookups");
}

// 3. fetchClineFreeModels — live API discovery with :free filtering (mock fetch)
{
  process.env.CLINE_API_KEY = "test-key";
  const originalFetch = globalThis.fetch;
  let capturedUrl = "";
  let capturedAuth = "";
  globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
    capturedUrl = String(url);
    const headers = (init?.headers ?? {}) as Record<string, string>;
    capturedAuth = headers["Authorization"] ?? headers["authorization"] ?? "";
    return new Response(
      JSON.stringify({
        data: [
          { id: "minimax/minimax-m3:free" },
          { id: "minimax/minimax-m3" },
          { id: "z-ai/glm-5.2:free" },
          { id: "anthropic/claude-sonnet-4-6" },
          { id: "google/gemma-4-31b-it:free" },
        ],
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    );
  }) as unknown as typeof fetch;

  try {
    const models = await fetchClineFreeModels();
    assert.equal(capturedUrl, `${CLINE_BASE_URL}/models`);
    assert.equal(capturedAuth, "Bearer test-key");
    assert.equal(models.length, 3);
    assert.equal(models[0]?.id, "google/gemma-4-31b-it:free");
    assert.equal(models[1]?.id, "minimax/minimax-m3:free");
    assert.equal(models[2]?.id, "z-ai/glm-5.2:free");
    assert.equal(models[0]?.contextWindow, 200_000);
    const m3 = models.find((m) => m.id === "minimax/minimax-m3:free");
    assert.equal(m3?.supportsImages, true);
    const glm = models.find((m) => m.id === "z-ai/glm-5.2:free");
    // glm is not in CLINE_SUPPORTS_IMAGES → supportsImages is omitted (undefined)
    assert.equal(glm?.supportsImages, undefined);
  } finally {
    delete process.env.CLINE_API_KEY;
    globalThis.fetch = originalFetch;
  }
  console.log("  ✅ fetchClineFreeModels → live API :free filtering");
}

// 4. fetchClineFreeModels — static fallback when no credentials
{
  delete process.env.CLINE_API_KEY;
  const originalPath = process.env.CLINE_CREDENTIALS_PATH;
  process.env.CLINE_CREDENTIALS_PATH = path.join(
    os.tmpdir(),
    `cline-nonexistent-${Date.now()}-${Math.random()}`,
  );
  try {
    const models = await fetchClineFreeModels();
    assert.equal(models.length, CLINE_FREE_MODEL_IDS.length);
    for (const m of models) {
      assert.ok(m.id.endsWith(":free"), `${m.id} should end with :free`);
      assert.equal(m.provider, "cline");
    }
    const sortedIds = models.map((m) => m.id).sort((a, b) => a.localeCompare(b));
    assert.deepEqual(models.map((m) => m.id), sortedIds);
  } finally {
    if (originalPath === undefined) {
      delete process.env.CLINE_CREDENTIALS_PATH;
    } else {
      process.env.CLINE_CREDENTIALS_PATH = originalPath;
    }
  }
  console.log("  ✅ fetchClineFreeModels → static fallback (no creds)");
}

// 5. fetchClineFreeModels — static fallback on network error
{
  process.env.CLINE_API_KEY = "test-key";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () => {
    throw new Error("network down");
  }) as unknown as typeof fetch;
  try {
    const models = await fetchClineFreeModels();
    assert.equal(models.length, CLINE_FREE_MODEL_IDS.length);
  } finally {
    delete process.env.CLINE_API_KEY;
    globalThis.fetch = originalFetch;
  }
  console.log("  ✅ fetchClineFreeModels → static fallback (network error)");
}

// 6. fetchClineFreeModels — static fallback on non-2xx
{
  process.env.CLINE_API_KEY = "test-key";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () => {
    return new Response("forbidden", { status: 403 });
  }) as unknown as typeof fetch;
  try {
    const models = await fetchClineFreeModels();
    assert.equal(models.length, CLINE_FREE_MODEL_IDS.length);
  } finally {
    delete process.env.CLINE_API_KEY;
    globalThis.fetch = originalFetch;
  }
  console.log("  ✅ fetchClineFreeModels → static fallback (non-2xx)");
}

// 7. loadClineCredential / saveClineCredential / clearClineCredential
{
  const tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), "cline-auth-test-"));
  const originalPath = process.env.CLINE_CREDENTIALS_PATH;
  const originalKey = process.env.CLINE_API_KEY;
  process.env.CLINE_CREDENTIALS_PATH = path.join(tmpDir, "cline-creds.json");
  delete process.env.CLINE_API_KEY;
  try {
    const before = await loadClineCredential();
    assert.equal(before, null);

    await saveClineCredential({ apiKey: "sk_test_123" });
    const loaded = await loadClineCredential();
    assert.deepEqual(loaded, { apiKey: "sk_test_123" });

    await clearClineCredential();
    const after = await loadClineCredential();
    assert.equal(after, null);

    await clearClineCredential();
  } finally {
    if (originalPath === undefined) delete process.env.CLINE_CREDENTIALS_PATH;
    else process.env.CLINE_CREDENTIALS_PATH = originalPath;
    if (originalKey === undefined) delete process.env.CLINE_API_KEY;
    else process.env.CLINE_API_KEY = originalKey;
    await fs.rm(tmpDir, { recursive: true, force: true });
  }
  console.log("  ✅ loadClineCredential / saveClineCredential / clearClineCredential");
}

// 7b. CLINE_API_KEY env var takes precedence over the file
{
  const tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), "cline-env-test-"));
  const originalPath = process.env.CLINE_CREDENTIALS_PATH;
  const originalKey = process.env.CLINE_API_KEY;
  process.env.CLINE_CREDENTIALS_PATH = path.join(tmpDir, "cline-creds.json");
  process.env.CLINE_API_KEY = "sk_env_wins";
  await saveClineCredential({ apiKey: "sk_file_loses" });
  try {
    const loaded = await loadClineCredential();
    assert.deepEqual(loaded, { apiKey: "sk_env_wins" });
  } finally {
    if (originalPath === undefined) delete process.env.CLINE_CREDENTIALS_PATH;
    else process.env.CLINE_CREDENTIALS_PATH = originalPath;
    if (originalKey === undefined) delete process.env.CLINE_API_KEY;
    else process.env.CLINE_API_KEY = originalKey;
    await fs.rm(tmpDir, { recursive: true, force: true });
  }
  console.log("  ✅ CLINE_API_KEY env var overrides stored file");
}

// 8. provider-registry catalog entry
{
  const cline = listProviders().find((p) => p.name === "cline");
  assert.ok(cline, "cline provider missing from registry");
  assert.equal(cline.displayName, "Cline");
  assert.equal(cline.authMethod, "api-key");
  assert.equal(cline.models.length, 18);
  for (const m of cline.models) {
    assert.equal(m.provider, "cline");
    assert.ok(m.id.endsWith(":free"), `${m.id} should end with :free`);
    assert.ok(typeof m.contextWindow === "number" && m.contextWindow > 0);
  }
  const nemotron = cline.models.find((m) => m.id === "nvidia/nemotron-3.5-lightning:free");
  assert.equal(nemotron?.contextWindow, 1_000_000);
  const m3 = cline.models.find((m) => m.id === "minimax/minimax-m3:free");
  assert.equal(m3?.contextWindow, 200_000);
  assert.equal(m3?.supportsImages, true);
  const ids = cline.models.map((m) => m.id);
  const sorted = [...ids].sort((a, b) => a.localeCompare(b));
  assert.deepEqual(ids, sorted);
  console.log("  ✅ provider-registry cline catalog entry (all 18 free models)");
}

// 8b. listModelsForProvider("cline")
{
  const models = listModelsForProvider("cline");
  assert.equal(models.length, 18);
  console.log("  ✅ listModelsForProvider('cline') returns 18 free models");
}

// 8c. Type-level: "cline" is a valid ProviderId literal
{
  const id: import("@console/types").ProviderId = "cline";
  assert.equal(id, "cline");
  const m: import("@console/types").Model = {
    id: "minimax/minimax-m3:free",
    provider: "cline",
    contextWindow: 200_000,
    supportsImages: true,
  };
  assert.equal(m.provider, "cline");
  console.log("  ✅ ProviderId union includes 'cline'");
}

// 9. Cline auth routes — status, login (good + bad), logout
{
  const { clineAuthRoutes } = await import("@/api/src/routes/cline.js");

  const tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), "cline-routes-test-"));
  const originalPath = process.env.CLINE_CREDENTIALS_PATH;
  const originalKey = process.env.CLINE_API_KEY;
  process.env.CLINE_CREDENTIALS_PATH = path.join(tmpDir, "cline-creds.json");
  delete process.env.CLINE_API_KEY;
  await clearClineCredential();

  const originalFetch = globalThis.fetch;

  try {
    // status: not logged in
    let res = await clineAuthRoutes.request("/cline/status", { method: "GET" });
    let body = (await res.json()) as { data: { loggedIn: boolean } };
    assert.equal(body.data.loggedIn, false);

    // login with bad key (401) → 400
    globalThis.fetch = (async () => new Response("nope", { status: 401 })) as unknown as typeof fetch;
    res = await clineAuthRoutes.request("/cline/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ apiKey: "sk_bad" }),
    });
    assert.equal(res.status, 400);

    // login with empty body → 400
    res = await clineAuthRoutes.request("/cline/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    });
    assert.equal(res.status, 400);

    // login with good key (200) → 200, credential saved
    let probeAuth = "";
    globalThis.fetch = (async (_url: string | URL, init?: RequestInit) => {
      const headers = (init?.headers ?? {}) as Record<string, string>;
      probeAuth = headers["Authorization"] ?? headers["authorization"] ?? "";
      return new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }) as unknown as typeof fetch;
    res = await clineAuthRoutes.request("/cline/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ apiKey: "sk_good" }),
    });
    assert.equal(res.status, 200);
    assert.equal(probeAuth, "Bearer sk_good");

    // status now reflects logged-in
    res = await clineAuthRoutes.request("/cline/status", { method: "GET" });
    body = (await res.json()) as { data: { loggedIn: boolean } };
    assert.equal(body.data.loggedIn, true);

    // logout clears
    res = await clineAuthRoutes.request("/cline/logout", { method: "POST" });
    body = (await res.json()) as { data: { loggedIn: boolean } };
    assert.equal(body.data.loggedIn, false);

    res = await clineAuthRoutes.request("/cline/status", { method: "GET" });
    body = (await res.json()) as { data: { loggedIn: boolean } };
    assert.equal(body.data.loggedIn, false);
  } finally {
    globalThis.fetch = originalFetch;
    if (originalPath === undefined) delete process.env.CLINE_CREDENTIALS_PATH;
    else process.env.CLINE_CREDENTIALS_PATH = originalPath;
    if (originalKey === undefined) delete process.env.CLINE_API_KEY;
    else process.env.CLINE_API_KEY = originalKey;
    await fs.rm(tmpDir, { recursive: true, force: true });
  }
  console.log("  ✅ Cline auth routes (status / login / logout)");
}

console.log("Cline Provider tests passed!\n");
