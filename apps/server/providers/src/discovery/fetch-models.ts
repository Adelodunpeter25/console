/**
 * Dynamic Model Discovery (`fetchAvailableModels`).
 * Mirrors oh-my-pi/packages/catalog/src/discovery/antigravity.ts.
 *
 * Hits the /v1internal:fetchAvailableModels endpoint with an OAuth token
 * to fetch live discoverable models for Gemini CLI and Antigravity.
 */
import type { Model, OAuthProviderId } from "@console/types";

import {
  ANTIGRAVITY_BASE_URL,
  GEMINI_BASE_URL,
  getAntigravityUserAgent,
  getGeminiCliUserAgent,
} from "../constants.js";

const DENYLIST = new Set(["chat_20706", "chat_23310"]);

export interface FetchAvailableModelsOptions {
  accessToken: string;
  provider: OAuthProviderId;
  baseUrl?: string;
  userAgent?: string;
  signal?: AbortSignal;
}

export interface DiscoveredApiModel {
  displayName?: string;
  supportsImages?: boolean;
  supportsThinking?: boolean;
  thinkingBudget?: number;
  maxTokens?: number;
  maxOutputTokens?: number;
  isInternal?: boolean;
}

export interface FetchAvailableModelsResponse {
  models?: Record<string, DiscoveredApiModel>;
}

function getDefaultUserAgent(provider: OAuthProviderId): string {
  return provider === "antigravity" ? getAntigravityUserAgent() : getGeminiCliUserAgent();
}

/**
 * Call /v1internal:fetchAvailableModels and return canonical Model objects.
 * Returns null if network request or authentication fails.
 */
export async function fetchAvailableModels(
  options: FetchAvailableModelsOptions,
): Promise<Model[] | null> {
  const defaultBase = options.provider === "antigravity" ? ANTIGRAVITY_BASE_URL : GEMINI_BASE_URL;
  const baseUrl = (options.baseUrl ?? defaultBase).replace(/\/+$/, "");
  const url = `${baseUrl}/v1internal:fetchAvailableModels`;
  const userAgent = options.userAgent ?? getDefaultUserAgent(options.provider);

  let response: Response;
  try {
    response = await fetch(url, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${options.accessToken}`,
        "Content-Type": "application/json",
        "User-Agent": userAgent,
      },
      body: JSON.stringify({}),
      signal: options.signal,
    });
  } catch {
    return null;
  }

  if (!response.ok) {
    return null;
  }

  let payload: FetchAvailableModelsResponse;
  try {
    payload = (await response.json()) as FetchAvailableModelsResponse;
  } catch {
    return null;
  }

  if (!payload.models || typeof payload.models !== "object") {
    return null;
  }

  const models: Model[] = [];

  for (const [modelId, meta] of Object.entries(payload.models)) {
    if (DENYLIST.has(modelId)) continue;
    if (meta.isInternal === true) continue;

    const contextWindow =
      typeof meta.maxTokens === "number" && meta.maxTokens > 0 ? meta.maxTokens : 200_000;

    models.push({
      id: modelId,
      provider: options.provider,
      contextWindow,
      ...(typeof meta.supportsImages === "boolean" ? { supportsImages: meta.supportsImages } : {}),
    });
  }

  // Sort model IDs deterministically
  models.sort((a, b) => a.id.localeCompare(b.id));

  return models;
}
