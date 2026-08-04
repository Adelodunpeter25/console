/**
 * Provider Registry & Hybrid Model Catalog.
 * Registers supported providers ("gemini", "antigravity") with dynamic
 * endpoint discovery via /v1internal:fetchAvailableModels (mirroring oh-my-pi).
 */

import {
  createAntigravityStreamFn,
  fetchAvailableModels,
  geminiStreamFn,
  loadCredential,
  refreshIfNeeded,
} from "../../../providers/src/index.js";
import type { StreamFn } from "../service/agent-loop.js";

import type { Model, ProviderCatalogEntry } from "../types/index.js";

export interface ProviderEntry extends ProviderCatalogEntry {
  getStreamFn: () => StreamFn;
}

export type { ProviderCatalogEntry } from "../types/index.js";

export const DEFAULT_GEMINI_MODELS: Model[] = [
  { id: "gemini-2.5-flash", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-2.5-flash-lite", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-2.5-pro", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3-flash", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3-flash-preview", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3-pro-preview", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3.1-pro-low", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3.1-pro-high", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3.5-flash", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3.5-flash-lite", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3.6-flash", provider: "gemini", contextWindow: 1_048_576 },
  { id: "gemini-3.6-flash-medium", provider: "gemini", contextWindow: 1_048_576 },
];

export const DEFAULT_ANTIGRAVITY_MODELS: Model[] = [
  { id: "gemini-2.5-flash", provider: "antigravity", contextWindow: 1_048_576 },
  { id: "gemini-2.5-pro", provider: "antigravity", contextWindow: 1_048_576 },
  { id: "gemini-3-flash", provider: "antigravity", contextWindow: 1_048_576 },
  { id: "gemini-3.1-pro-low", provider: "antigravity", contextWindow: 1_048_576 },
  { id: "gemini-3.1-pro-high", provider: "antigravity", contextWindow: 1_048_576 },
  { id: "gemini-3.5-flash-low", provider: "antigravity", contextWindow: 1_048_576 },
  { id: "gemini-3.6-flash-medium", provider: "antigravity", contextWindow: 1_048_576 },
  { id: "claude-sonnet-4-6", provider: "antigravity", contextWindow: 250_000 },
  { id: "claude-opus-4-6-thinking", provider: "antigravity", contextWindow: 250_000 },
];

export const PROVIDER_CATALOG: Record<"gemini" | "antigravity", ProviderEntry> = {
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
};

export function listProviders(): ProviderCatalogEntry[] {
  return Object.values(PROVIDER_CATALOG).map(({ getStreamFn: _getStreamFn, ...rest }) => rest);
}

export function getProvider(name: string): ProviderEntry | undefined {
  return PROVIDER_CATALOG[name as "gemini" | "antigravity"];
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
 * Dynamically fetch models from the provider endpoint via /v1internal:fetchAvailableModels.
 * Updates the provider's cached model list if successful.
 * Falls back to bundled static models if offline, unauthenticated, or on network error.
 */
export async function fetchModelsForProvider(
  providerName: "gemini" | "antigravity",
  signal?: AbortSignal,
): Promise<Model[]> {
  const provider = getProvider(providerName);
  if (!provider) return [];

  try {
    const rawCred = await loadCredential(providerName);
    const cred = await refreshIfNeeded(rawCred, providerName, signal);

    const discovered = await fetchAvailableModels({
      accessToken: cred.accessToken,
      provider: providerName,
      signal,
    });

    if (discovered && discovered.length > 0) {
      provider.models = discovered;
      return discovered;
    }
  } catch {
    // Network/auth error — fall back to static models below
  }

  // If dynamic fetch failed or yielded no models, return static/cached models
  const staticFallback = providerName === "gemini" ? DEFAULT_GEMINI_MODELS : DEFAULT_ANTIGRAVITY_MODELS;
  if (!provider.models || provider.models.length === 0) {
    provider.models = staticFallback;
  }

  return provider.models;
}
