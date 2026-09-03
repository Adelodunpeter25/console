/**
 * OpenCode Zen model discovery.
 * GET https://opencode.ai/zen/v1/models, filter to free-tier model ids.
 * Falls back to the static free list on network/parse errors.
 */
import type { Model } from "@console/types";
import {
  OPENCODE_BASE_URL,
  OPENCODE_CONTEXT_WINDOW,
  OPENCODE_FREE_MODEL_IDS,
  OPENCODE_USER_AGENT,
} from "./constants.js";

interface OpenCodeModelsResponse {
  data?: Array<{ id: string }>;
}

/** True when a model id is on the free tier. Matches the known free IDs
 *  (including "big-pickle" which doesn't follow the `-free` suffix convention)
 *  plus any model whose id ends with "-free", so newly added free models are
 *  discovered automatically without a code change. */
export function isOpencodeFreeModelId(id: string): boolean {
  if (id.endsWith("-free")) return true;
  return OPENCODE_FREE_MODEL_IDS.includes(
    id as (typeof OPENCODE_FREE_MODEL_IDS)[number],
  );
}

export async function fetchOpencodeFreeModels(
  signal?: AbortSignal,
): Promise<Model[]> {
  try {
    const response = await fetch(`${OPENCODE_BASE_URL}/models`, {
      method: "GET",
      headers: {
        Accept: "application/json",
        "User-Agent": OPENCODE_USER_AGENT,
      },
      signal,
    });

    if (!response.ok) return fallbackOpencodeModels();

    const payload = (await response.json()) as OpenCodeModelsResponse;
    const ids = (payload.data ?? []).map((m) => m.id).filter(isOpencodeFreeModelId);
    if (ids.length === 0) return fallbackOpencodeModels();

    return ids.map((id) => ({
      id,
      provider: "opencode",
      contextWindow: OPENCODE_CONTEXT_WINDOW,
    }));
  } catch {
    return fallbackOpencodeModels();
  }
}

function fallbackOpencodeModels(): Model[] {
  return OPENCODE_FREE_MODEL_IDS.map((id) => ({
    id,
    provider: "opencode",
    contextWindow: OPENCODE_CONTEXT_WINDOW,
  }));
}
