/**
 * Claude model discovery — `GET /v1/models` with the subscription OAuth
 * token. Returns null on any failure so the registry falls back to the
 * static model seed.
 */
import { claudeModelsUrl } from "./constants.js";
import { loadClaudeCredential, refreshClaudeIfNeeded } from "./oauth.js";

export interface ClaudeDiscoveredModel {
  id: string;
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
  const payload = (await response.json()) as { data?: Array<{ id?: string }> };
  const models = (payload.data ?? []).flatMap((entry) => {
    if (!entry.id) return [];
    return [{ id: entry.id }];
  });
  return models.length > 0 ? models : null;
}
