/**
 * Unit Tests for Dynamic Model Discovery (fetchAvailableModels) & ProviderRegistry.
 * Operates offline — uses mock fetch response (0 LLM credits consumed).
 */
import assert from "node:assert/strict";
import {
  fetchModelsForProvider,
  listModelsForProvider,
} from "../server/agent/src/commands/provider-registry.js";
import { fetchAvailableModels } from "../server/providers/src/discovery/fetch-models.js";

console.log("Running Model Discovery tests...");

// 1. Test fetchAvailableModels with mock response
{
  const mockApiResponse = {
    models: {
      "gemini-3.1-pro-low": {
        displayName: "Gemini 3.1 Pro",
        supportsImages: true,
        supportsThinking: true,
        maxTokens: 200_000,
        maxOutputTokens: 65_535,
        isInternal: false,
      },
      "claude-sonnet-4-6": {
        displayName: "Claude 3.7 Sonnet",
        supportsImages: true,
        maxTokens: 200_000,
        maxOutputTokens: 64_000,
        isInternal: false,
      },
      chat_20706: {
        displayName: "Internal Model",
        isInternal: true,
      },
    },
  };

  // Mock global fetch for testing parsing
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () => {
    return new Response(JSON.stringify(mockApiResponse), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;

  try {
    const models = await fetchAvailableModels({
      accessToken: "mock-token",
      provider: "antigravity",
    });

    assert.ok(models);
    assert.equal(models.length, 2); // Internal chat_20706 filtered out
    assert.equal(models[0]?.id, "claude-sonnet-4-6");
    assert.equal(models[1]?.id, "gemini-3.1-pro-low");
    console.log("  ✅ fetchAvailableModels parsing & denylist filtering");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 2. Test provider-registry fallback on network/auth failure
{
  const models = listModelsForProvider("antigravity");
  assert.ok(models.length > 0);
  assert.ok(models.some((m) => m.id === "gemini-3.1-pro-low"));

  // Calling fetchModelsForProvider without valid token returns cached/bundled models
  const fetched = await fetchModelsForProvider("antigravity");
  assert.ok(fetched.length > 0);
  console.log("  ✅ provider-registry dynamic fetch & bundled static fallbacks");
}

console.log("Model Discovery tests passed!\n");
