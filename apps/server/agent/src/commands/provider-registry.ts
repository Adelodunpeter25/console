/**
 * Provider Registry & Hybrid Model Catalog.
 * Registers supported providers ("antigravity", "codex", "cline", "devin", "claude") with dynamic
 * endpoint discovery via /v1internal:fetchAvailableModels (mirroring oh-my-pi).
 */

import {
  createAntigravityStreamFn,
  fetchAvailableModels,
  fetchClineFreeModels,
  fetchDevinModels,
  loadCredential,
  refreshIfNeeded,
  codexStreamFn,
  codexCredentialExists,
  loadCodexCredential,
  refreshCodexIfNeeded,
  clineStreamFn,
  CLINE_FREE_MODEL_IDS,
  getClineContextWindow,
  getClineSupportsImages,
  devinStreamFn,
  claudeStreamFn,
  claudeCredentialExists,
  fetchClaudeModels,
} from "@/providers/src/index.js";
import { codexModelsUrl } from "@/providers/src/codex/constants.js";
import type { StreamFn } from "@/agent/src/service/agent-loop.js";

import type { Model, ProviderCatalogEntry, ProviderId, ThinkingLevel } from "@/agent/src/types/index.js";

export interface ProviderEntry extends ProviderCatalogEntry {
  getStreamFn: () => StreamFn;
}

export type { ProviderCatalogEntry } from "@/agent/src/types/index.js";

export const DEFAULT_FALLBACK_MODEL = "claude-opus-4-6-thinking";

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

const GEMINI_THINKING_LEVELS: ThinkingLevel[] = ["minimal", "low", "medium", "high"];

const CODEX_THINKING_LEVELS: ThinkingLevel[] = ["none", "minimal", "low", "medium", "high", "xhigh", "max"];

const CLAUDE_THINKING_LEVELS: ThinkingLevel[] = ["low", "medium", "high", "xhigh", "max"];

/**
 * Best-effort thinking-level support for a model that either isn't in the
 * in-memory catalog yet (not-yet-discovered id) or is a seed entry, keyed by
 * provider + naming convention. Used to avoid `buildRunModel`/`resolveRoleModel`
 * producing a bare `Model` with no thinking fields, which `validate-thinking.ts`
 * would then reject as "does not support thinking levels" even when it does.
 */
export function inferThinkingLevels(
  provider: ProviderId,
  modelId: string,
): Pick<Model, "supportedThinkingLevels" | "defaultThinkingLevel"> {
  if (provider === "codex") {
    return { supportedThinkingLevels: CODEX_THINKING_LEVELS, defaultThinkingLevel: "low" };
  }
  if (provider === "claude") {
    return { supportedThinkingLevels: CLAUDE_THINKING_LEVELS, defaultThinkingLevel: "low" };
  }
  if (provider === "antigravity" && (modelId.startsWith("gemini-") || modelId.startsWith("claude-"))) {
    return { supportedThinkingLevels: GEMINI_THINKING_LEVELS, defaultThinkingLevel: "low" };
  }
  return {};
}

export const DEFAULT_ANTIGRAVITY_MODELS: Model[] = AVAILABLE_MODELS.map((id) => ({
  id,
  provider: "antigravity",
  // Offline seed mirrors measured fetchAvailableModels maxTokens:
  // claude-* 250k, gpt-oss-120b 128k, gemini-* 1M. Live discovery overwrites.
  contextWindow: id.startsWith("claude-") ? 250_000 : id.startsWith("gpt-oss-") ? 131_072 : 1_048_576,
  ...inferThinkingLevels("antigravity", id),
}));

export const DEFAULT_CODEX_MODELS: Model[] = [
  "gpt-5.6-terra",
  "gpt-5.6-luna",
  "gpt-5.5",
  "gpt-5.4-mini",
].map((id) => ({
  id,
  provider: "codex" as const,
  contextWindow: 272_000,
  supportsImages: true,
  supportedThinkingLevels: CODEX_THINKING_LEVELS,
  defaultThinkingLevel: "low" as const,
}));

export const DEFAULT_CLINE_MODELS: Model[] = [...CLINE_FREE_MODEL_IDS]
  .map((id) => ({
    id,
    provider: "cline" as const,
    contextWindow: getClineContextWindow(id),
    ...(getClineSupportsImages(id) ? { supportsImages: true } : {}),
  }))
  .sort((a, b) => a.id.localeCompare(b.id));

// Devin's model catalog is fully server-driven via `GetCliModelConfigs` and is
// only available after the user logs in. There is no offline seed — before
// auth the picker shows zero Devin models, on first auth it gets the full list.
export const DEFAULT_DEVIN_MODELS: Model[] = [];

// Offline seed — context windows measured from GET /v1/models
// (max_input_tokens). Refreshed from the live list after login.
export const DEFAULT_CLAUDE_MODELS: Model[] = [
  { id: "claude-sonnet-4-5", contextWindow: 1_000_000 },
  { id: "claude-opus-4-6", contextWindow: 1_000_000 },
  { id: "claude-sonnet-4-6", contextWindow: 1_000_000 },
  { id: "claude-haiku-4-5", contextWindow: 200_000 },
].map(({ id, contextWindow }) => ({
  id,
  provider: "claude" as const,
  contextWindow,
  supportsImages: true,
  supportedThinkingLevels: CLAUDE_THINKING_LEVELS,
  defaultThinkingLevel: "low" as const,
}));

/** Providers that are temporarily disabled (kept in code but hidden from catalog). */
const DISABLED_PROVIDERS = new Set<ProviderId>(["cline", "devin"]);

export const PROVIDER_CATALOG: Record<ProviderId, ProviderEntry> = {
  antigravity: {
    name: "antigravity",
    displayName: "Google Antigravity",
    description: "Daily Cloud Code Assist endpoint with Antigravity session envelope",
    authMethod: "oauth",
    models: DEFAULT_ANTIGRAVITY_MODELS,
    getStreamFn: () => createAntigravityStreamFn(),
  },
  codex: {
    name: "codex",
    displayName: "OpenAI Codex",
    description: "ChatGPT subscription models through the Codex Responses API",
    authMethod: "oauth",
    models: DEFAULT_CODEX_MODELS,
    getStreamFn: () => codexStreamFn,
  },
  cline: {
    name: "cline",
    displayName: "Cline",
    description: "Free models via the Cline OpenAI-compatible gateway",
    authMethod: "api-key",
    models: DEFAULT_CLINE_MODELS,
    getStreamFn: () => clineStreamFn,
  },
  devin: {
    name: "devin",
    displayName: "Devin",
    description: "Codeium/Windsurf Cascade with PKCE OAuth + Connect/protobuf streaming",
    authMethod: "oauth",
    models: DEFAULT_DEVIN_MODELS,
    getStreamFn: () => devinStreamFn,
  },
  claude: {
    name: "claude",
    displayName: "Claude",
    description: "Claude Pro/Max subscription models through the Anthropic Messages API",
    authMethod: "oauth",
    models: DEFAULT_CLAUDE_MODELS,
    getStreamFn: () => claudeStreamFn,
  },
};

export function listProviders(): ProviderCatalogEntry[] {
  return Object.values(PROVIDER_CATALOG)
    .filter((p) => !DISABLED_PROVIDERS.has(p.name))
    .map(({ getStreamFn: _getStreamFn, ...rest }) => rest);
}

export function getProvider(name: string): ProviderEntry | undefined {
  if (DISABLED_PROVIDERS.has(name as ProviderId)) return undefined;
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
  if (DISABLED_PROVIDERS.has(providerName)) return [];
  const provider = getProvider(providerName);
  if (!provider) return [];

  try {
    let discovered: Model[] | null = null;

    if (providerName === "devin") {
      const discoveredDevin = await fetchDevinModels(fetch, signal);
      if (discoveredDevin) {
        discovered = discoveredDevin.map((m) => ({
          id: m.id,
          provider: "devin" as const,
          contextWindow: m.contextWindow ?? 200_000,
          ...(m.supportsImages ? { supportsImages: true } : {}),
        }));
      }
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
            supportedThinkingLevels: CODEX_THINKING_LEVELS,
            defaultThinkingLevel: "low" as const,
          }];
        });
      }
    } else if (providerName === "cline") {
      discovered = await fetchClineFreeModels(signal);
    } else if (providerName === "claude") {
      if (!(await claudeCredentialExists())) throw new Error("Claude is not logged in");
      const discoveredClaude = await fetchClaudeModels(signal);
      if (discoveredClaude) {
        discovered = discoveredClaude.map((m) => ({
          id: m.id,
          provider: "claude" as const,
          contextWindow: m.contextWindow ?? 200_000,
          ...(m.supportsImages ? { supportsImages: true } : {}),
          supportedThinkingLevels: CLAUDE_THINKING_LEVELS,
          defaultThinkingLevel: "low" as const,
        }));
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
  // (Devin has no static fallback — its catalog is server-driven and the picker
  // stays empty until discovery succeeds against `GetCliModelConfigs`).
  const staticFallback =
    providerName === "codex"
      ? DEFAULT_CODEX_MODELS
      : providerName === "cline"
        ? DEFAULT_CLINE_MODELS
        : providerName === "claude"
          ? DEFAULT_CLAUDE_MODELS
          : DEFAULT_ANTIGRAVITY_MODELS;
  if (!provider.models || provider.models.length === 0) {
    provider.models = staticFallback;
  }

  return provider.models;
}
