/**
 * Claude model discovery — `GET /v1/models` with the subscription OAuth
 * token. Returns null on any failure so the registry falls back to the
 * static model seed.
 */
import { claudeModelsUrl } from "./constants.js";
import { loadClaudeCredential, refreshClaudeIfNeeded } from "./oauth.js";

export interface ClaudeDiscoveredModel {
  id: string;
  contextWindow?: number;
  supportsImages?: boolean;
}

interface ClaudeModelsApiEntry {
  id?: string;
  /** Maximum input context window — nullable, often omitted. */
  max_input_tokens?: number | null;
  capabilities?: {
    image_input?: { supported?: boolean };
  } | null;
}

export async function fetchClaudeModels(signal?: AbortSignal): Promise<ClaudeDiscoveredModel[] | null> {
  const credential = await refreshClaudeIfNeeded(await loadClaudeCredential());
  const response = await fetch(claudeModelsUrl(), {
    headers: {
      Authorization: `Bearer ${credential.accessToken}`,
      "anthropic-version": "2023-06-01",
      "anthropic-beta": "oauth-2025-04-20",
      Accept: "application/json",
    },
    signal,
  });
  if (!response.ok) return null;
  const payload = (await response.json()) as { data?: ClaudeModelsApiEntry[] };
  const models = (payload.data ?? []).flatMap((entry) => {
    if (!entry.id) return [];
    const contextWindow =
      typeof entry.max_input_tokens === "number" && entry.max_input_tokens > 0
        ? entry.max_input_tokens
        : undefined;
    const supportsImages = entry.capabilities?.image_input?.supported;
    return [{ id: entry.id, ...(contextWindow !== undefined ? { contextWindow } : {}), ...(supportsImages !== undefined ? { supportsImages } : {}) }];
  });
  return models.length > 0 ? models : null;
}
