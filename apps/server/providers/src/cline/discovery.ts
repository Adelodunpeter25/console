/**
 * Cline model discovery.
 * GET /v1/models, filter to free-tier ids (suffix ":free").
 * Falls back to the static CLINE_FREE_MODEL_IDS on network/parse errors.
 *
 * No allowlist filter — any :free ID returned by the API is registered.
 * Cline is the source of truth for what counts as free.
 */
import type { Model } from "@console/types";
import {
  CLINE_BASE_URL,
  CLINE_CONTEXT_WINDOWS,
  CLINE_CONTEXT_WINDOW_DEFAULT,
  CLINE_FREE_MODEL_IDS,
  CLINE_SUPPORTS_IMAGES,
} from "./constants.js";
import { loadClineCredential } from "./auth.js";

interface ClineModelsResponse {
  data?: Array<{ id: string }>;
}

/** True when a model id is on the free tier. Matches the :free suffix convention. */
export function isClineFreeModelId(id: string): boolean {
  return id.endsWith(":free");
}

export function getClineContextWindow(id: string): number {
  return CLINE_CONTEXT_WINDOWS[id] ?? CLINE_CONTEXT_WINDOW_DEFAULT;
}

export function getClineSupportsImages(id: string): boolean {
  return CLINE_SUPPORTS_IMAGES[id] ?? false;
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

    return ids
      .map((id) => ({
        id,
        provider: "cline" as const,
        contextWindow: getClineContextWindow(id),
        ...(getClineSupportsImages(id) ? { supportsImages: true } : {}),
      }))
      .sort((a, b) => a.id.localeCompare(b.id));
  } catch {
    return fallbackClineModels();
  }
}

function fallbackClineModels(): Model[] {
  return [...CLINE_FREE_MODEL_IDS]
    .map((id) => ({
      id,
      provider: "cline" as const,
      contextWindow: getClineContextWindow(id),
      ...(getClineSupportsImages(id) ? { supportsImages: true } : {}),
    }))
    .sort((a, b) => a.id.localeCompare(b.id));
}