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

export const PROVIDER_CATALOG: Record<"gemini" | "antigravity", ProviderEntry> = {
  gemini: {
    name: "gemini",
    displayName: "Google Gemini CLI",
    description: "Cloud Code Assist REST endpoint with Gemini CLI OAuth",
    models: [],
    getStreamFn: () => geminiStreamFn,
  },
  antigravity: {
    name: "antigravity",
    displayName: "Google Antigravity",
    description: "Daily Cloud Code Assist endpoint with Antigravity session envelope",
    models: [],
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
 * Returns an empty array if offline, unauthenticated, or on network error — no fallbacks.
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
    // Network/auth error — no fallback models to offer
  }

  return [];
}
