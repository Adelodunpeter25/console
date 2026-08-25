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
  codebuffStreamFn,
  CODEBUFF_MODEL_SPECS,
  codexStreamFn,
  codexCredentialExists,
  loadCodexCredential,
  refreshCodexIfNeeded,
  OPENCODE_FREE_MODEL_IDS,
} from "@/providers/src/index.js";
import { codexModelsUrl } from "@/providers/src/codex/constants.js";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";

import type { Model, ProviderCatalogEntry, ProviderId } from "@/agent/src/types/index.js";

export interface ProviderEntry extends ProviderCatalogEntry {
  getStreamFn: () => StreamFn;
}

export type { ProviderCatalogEntry } from "@/agent/src/types/index.js";

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

export const DEFAULT_OPENCODE_MODELS: Model[] = OPENCODE_FREE_MODEL_IDS.map((id) => ({
  id,
  provider: "opencode" as const,
  contextWindow: 200_000,
}));

export const DEFAULT_CODEBUFF_MODELS: Model[] = CODEBUFF_MODEL_SPECS.map(
  (spec) => ({
    id: spec.id,
    provider: "codebuff" as const,
    contextWindow: spec.contextWindow,
    ...(spec.multimodal ? { supportsImages: true } : {}),
  }),
);

export const DEFAULT_CODEX_MODELS: Model[] = [
  "gpt-5.6-terra",
  "gpt-5.6-luna",
  "gpt-5.5",
  "gpt-5.4-mini",
].map((id) => ({ id, provider: "codex" as const, contextWindow: 272_000, supportsImages: true }));

export const PROVIDER_CATALOG: Record<ProviderId, ProviderEntry> = {
  gemini: {
    name: "gemini",
    displayName: "Google Gemini CLI",
    description: "Cloud Code Assist REST endpoint with Gemini CLI OAuth",
    authMethod: "oauth",
    models: DEFAULT_GEMINI_MODELS,
    getStreamFn: () => geminiStreamFn,
  },
  antigravity: {
    name: "antigravity",
    displayName: "Google Antigravity",
    description: "Daily Cloud Code Assist endpoint with Antigravity session envelope",
    authMethod: "oauth",
    models: DEFAULT_ANTIGRAVITY_MODELS,
    getStreamFn: () => createAntigravityStreamFn(),
  },
  opencode: {
    name: "opencode",
    displayName: "OpenCode Zen",
    description: "Free OpenAI-compatible endpoint (opencode.ai/zen)",
    authMethod: "none",
    models: DEFAULT_OPENCODE_MODELS,
    getStreamFn: () => opencodeStreamFn,
  },
  codebuff: {
    name: "codebuff",
    displayName: "Codebuff (Freebuff)",
    description: "Free-tier models served by codebuff.com (DeepSeek, MiMo, Kimi, MiniMax)",
    authMethod: "device-code",
    models: DEFAULT_CODEBUFF_MODELS,
    getStreamFn: () => codebuffStreamFn,
  },
  codex: {
    name: "codex",
    displayName: "OpenAI Codex",
    description: "ChatGPT subscription models through the Codex Responses API",
    authMethod: "oauth",
    models: DEFAULT_CODEX_MODELS,
    getStreamFn: () => codexStreamFn,
  },
};

export function listProviders(): ProviderCatalogEntry[] {
  return Object.values(PROVIDER_CATALOG).map(({ getStreamFn: _getStreamFn, ...rest }) => rest);
}

export function getProvider(name: string): ProviderEntry | undefined {
  return PROVIDER_CATALOG[name as ProviderId];
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
  providerName: ProviderId,
  signal?: AbortSignal,
): Promise<Model[]> {
  const provider = getProvider(providerName);
  if (!provider) return [];

  try {
    let discovered: Model[] | null = null;

    if (providerName === "opencode") {
      discovered = await fetchOpencodeFreeModels(signal);
    } else if (providerName === "codebuff") {
      // Codebuff is device-code auth with its own backend, not OAuth token
      // discovery — its catalog ships statically (fallback below).
      throw new Error("Codebuff models are served from the bundled catalog");
    } else if (providerName === "codex") {
      if (!(await codexCredentialExists())) throw new Error("Codex is not logged in");
      const cred = await refreshCodexIfNeeded(await loadCodexCredential());
      const response = await fetch(codexModelsUrl(), {
        headers: {
          Authorization: `Bearer ${cred.accessToken}`,
          "chatgpt-account-id": cred.accountId,
          "OpenAI-Beta": "responses=experimental",
          originator: "pi",
          version: "0.144.1",
          Accept: "application/json",
        },
        signal,
      });
      if (response.ok) {
        const payload = (await response.json()) as { models?: Array<{ slug?: string; id?: string; context_window?: number; input_modalities?: string[] }> };
        discovered = (payload.models ?? []).flatMap((entry) => {
          const id = entry.slug ?? entry.id;
          if (!id) return [];
          return [{
            id,
            provider: "codex" as const,
            contextWindow: entry.context_window ?? 272_000,
            ...(entry.input_modalities?.includes("image") ? { supportsImages: true } : {}),
          }];
        });
      }
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
      : providerName === "codebuff"
          ? DEFAULT_CODEBUFF_MODELS
          : providerName === "codex"
            ? DEFAULT_CODEX_MODELS
          : DEFAULT_ANTIGRAVITY_MODELS;
  if (!provider.models || provider.models.length === 0) {
    provider.models = staticFallback;
  }

  return provider.models;
}
