/**
 * Provider Registry & Hybrid Model Catalog.
 * Registers supported providers ("gemini", "antigravity") with dynamic
 * endpoint discovery via /v1internal:fetchAvailableModels (mirroring oh-my-pi).
 */

import {
  createAntigravityStreamFn,
  fetchAvailableModels,
  fetchOpencodeFreeModels,
  geminiStreamFn,
  loadCredential,
  opencodeStreamFn,
  refreshIfNeeded,
} from "../../../providers/src/index.js";
import type { StreamFn } from "../service/agent-loop.js";

import type { Model, ProviderCatalogEntry } from "../types/index.js";

export interface ProviderEntry extends ProviderCatalogEntry {
  getStreamFn: () => StreamFn;
}

export type { ProviderCatalogEntry } from "../types/index.js";

export const DEFAULT_FALLBACK_MODEL = "gemini-3-flash";

export const AVAILABLE_MODELS = [
  "claude-opus-4-6-thinking",
  "claude-sonnet-4-6",
  "gemini-3.1-pro-high",
  "gemini-3.1-pro-low",
  "gemini-3-flash",
  "gemini-3-flash-agent",
  "gemini-3.5-flash-low",
  "gpt-oss-120b-medium",
] as const;

export const DEFAULT_GEMINI_MODELS: Model[] = [
  { id: "gemini-3.1-pro-preview", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3-pro-preview", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-2.5-pro", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-2.5-flash", provider: "gemini", contextWindow: 1_048_576 },
];

export const DEFAULT_ANTIGRAVITY_MODELS: Model[] = AVAILABLE_MODELS.map((id) => ({
  id,
  provider: "antigravity",
  contextWindow: id.startsWith("claude-") ? 250_000 : 1_048_576,
}));

export const DEFAULT_OPENCODE_MODELS: Model[] = [
  "big-pickle",
  "deepseek-v4-flash-free",
  "mimo-v2.5-free",
  "ling-3.0-flash-free",
  "nemotron-3-ultra-free",
  "north-mini-code-free",
  "laguna-s-2.1-free",
  "longcat-2.0-free",
].map((id) => ({ id, provider: "opencode", contextWindow: 128_000 }));

export const PROVIDER_CATALOG: Record<
  "gemini" | "antigravity" | "opencode",
  ProviderEntry
> = {
  gemini: {
    name: "gemini",
    displayName: "Google Gemini CLI",
    description: "Cloud Code Assist REST endpoint with Gemini CLI OAuth",
    models: DEFAULT_GEMINI_MODELS,
    getStreamFn: () => geminiStreamFn,
  },
  antigravity: {
    name: "antigravity",
    displayName: "Google Antigravity",
    description: "Daily Cloud Code Assist endpoint with Antigravity session envelope",
    models: DEFAULT_ANTIGRAVITY_MODELS,
    getStreamFn: () => createAntigravityStreamFn(),
  },
  opencode: {
    name: "opencode",
    displayName: "OpenCode Zen",
    description: "Free OpenAI-compatible endpoint (opencode.ai/zen)",
    models: DEFAULT_OPENCODE_MODELS,
    getStreamFn: () => opencodeStreamFn,
  },
};

export function listProviders(): ProviderCatalogEntry[] {
  return Object.values(PROVIDER_CATALOG).map(({ getStreamFn: _getStreamFn, ...rest }) => rest);
}

export function getProvider(name: string): ProviderEntry | undefined {
  return PROVIDER_CATALOG[name as "gemini" | "antigravity" | "opencode"];
}

export function listModelsForProvider(name: string): Model[] {
  const provider = getProvider(name);
  return provider ? provider.models : [];
}

export function findModelInProvider(providerName: string, modelId: string): Model | undefined {
  const models = listModelsForProvider(providerName);
  return models.find((m) => m.id.toLowerCase() === modelId.toLowerCase());
}

/**
 * Dynamically fetch models from the provider endpoint via /v1internal:fetchAvailableModels
 * (or the Zen /models endpoint). Updates the provider's cached model list if successful.
 * Falls back to bundled static models if offline, unauthenticated, or on network error.
 */
export async function fetchModelsForProvider(
  providerName: "gemini" | "antigravity" | "opencode",
  signal?: AbortSignal,
): Promise<Model[]> {
  const provider = getProvider(providerName);
  if (!provider) return [];

  try {
    let discovered: Model[] | null = null;

    if (providerName === "opencode") {
      discovered = await fetchOpencodeFreeModels(signal);
    } else {
      const rawCred = await loadCredential(providerName);
      const cred = await refreshIfNeeded(rawCred, providerName, signal);

      discovered = await fetchAvailableModels({
        accessToken: cred.accessToken,
        provider: providerName,
        signal,
      });
    }

    if (discovered && discovered.length > 0) {
      provider.models = discovered;
      return discovered;
    }
  } catch {
    // Network/auth error — fall back to static models below
  }

  // If dynamic fetch failed or yielded no models, return static/cached models
  const staticFallback =
    providerName === "gemini"
      ? DEFAULT_GEMINI_MODELS
      : providerName === "opencode"
        ? DEFAULT_OPENCODE_MODELS
        : DEFAULT_ANTIGRAVITY_MODELS;
  if (!provider.models || provider.models.length === 0) {
    provider.models = staticFallback;
  }

  return provider.models;
}
