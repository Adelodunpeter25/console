/**
 * Unit Tests for Dynamic Model Discovery (fetchAvailableModels) & ProviderRegistry.
 * Operates offline — uses mock fetch response (0 LLM credits consumed).
 */
import assert from "node:assert/strict";
import {
  fetchModelsForProvider,
  listModelsForProvider,
  inferThinkingLevels,
} from "@/agent/src/commands/provider-registry.js";
import { fetchAvailableModels } from "@/providers/src/discovery/fetch-models.js";
import { buildRunModel } from "@/api/src/services/run/run-tools.js";

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
  }) as unknown as typeof fetch;

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

// 3. Test ProviderService favorites prioritization
{
  const { ProviderService } = await import("@/api/src/services/provider.service.js");
  const mockStorage: any = {
    listModelFavorites: () => [
      { provider: "antigravity", modelId: "gemini-3.1-pro-low" },
    ],
  };

  const service = new ProviderService(mockStorage);
  const providers = service.getProviders();
  const antigravity = providers.find((p) => p.name === "antigravity");
  assert.ok(antigravity);
  // Originally 4th in AVAILABLE_MODELS, now sorted to 1st
  assert.equal(antigravity.models[0]?.id, "gemini-3.1-pro-low");

  const models = await service.getModels("antigravity");
  assert.ok(models.length > 0);
  assert.equal(models[0]?.id, "gemini-3.1-pro-low");
  console.log("  ✅ ProviderService models list prioritizes favorites first");
}

// 4. Regression: thinking-level metadata must survive for models proxied
// through antigravity under a claude-* id, and for ids missing from the
// in-memory catalog entirely — both previously produced a bare Model with
// no supportedThinkingLevels, which validate-thinking.ts then rejected.
{
  const claudeUnderAntigravity = inferThinkingLevels("antigravity", "claude-sonnet-4-6");
  assert.ok(claudeUnderAntigravity.supportedThinkingLevels?.length);
  assert.equal(claudeUnderAntigravity.defaultThinkingLevel, "low");

  const unknownGeminiModel = buildRunModel("antigravity", "gemini-3.7-flash-tiered");
  assert.ok(unknownGeminiModel.supportedThinkingLevels?.length);
  assert.equal(unknownGeminiModel.defaultThinkingLevel, "low");

  const knownClaudeModel = buildRunModel("claude", "claude-sonnet-4-6");
  assert.ok(knownClaudeModel.supportedThinkingLevels?.length);
  console.log("  ✅ inferThinkingLevels/buildRunModel preserve thinking support for unseeded & cross-provider ids");
}

console.log("Model Discovery tests passed!\n");
