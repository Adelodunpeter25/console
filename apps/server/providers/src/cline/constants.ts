/**
 * Cline provider — OpenAI-compatible chat completions endpoint.
 * https://docs.cline.bot/api/overview
 *
 * Auth: Bearer token in Authorization header. No OAuth.
 *   Get key: app.cline.bot > Settings > API Keys.
 *   Env var: CLINE_API_KEY overrides stored credential.
 *
 * Free-tier filter: model IDs ending in ":free".
 * v1 ships all 18 free IDs discovered at runtime from /v1/models.
 */
export const CLINE_BASE_URL = "https://api.cline.bot/api/v1";

/**
 * All 18 free Cline model IDs (verified live). Used as the static fallback
 * when the live API is unreachable. New free models added upstream surface
 * automatically via discovery; this list is just an offline safety net.
 */
export const CLINE_FREE_MODEL_IDS = [
  "cohere/north-mini-code:free",
  "dots-studio/dots-3-note-preview:free",
  "google/gemma-4-26b-a4b-it:free",
  "google/gemma-4-31b-it:free",
  "inclusionai/ling-3.0-flash-fin:free",
  "liquid/lfm-2.5-2.6b:free",
  "minimax/minimax-m2.7:free",
  "minimax/minimax-m3:free",
  "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free",
  "nvidia/nemotron-3-super-120b-a12b:free",
  "nvidia/nemotron-3.5-content-safety:free",
  "nvidia/nemotron-3.5-lightning:free",
  "nvidia/nemotron-3-ultra-550b-a55b:free",
  "poolside/laguna-s-2.1:free",
  "poolside/laguna-xs-2.1:free",
  "thinkingmachines/inkling-small:free",
  "thinkingmachines/inkling:free",
  "z-ai/glm-5.2:free",
] as const;

/**
 * Best-effort context windows per free model ID. /v1/models does not return
 * this field (only {id, object, created, owned_by}), so values are inferred
 * from each model's known family capabilities. Fallback: CLINE_CONTEXT_WINDOW_DEFAULT.
 */
export const CLINE_CONTEXT_WINDOWS: Record<string, number> = {
  "cohere/north-mini-code:free": 200_000,
  "dots-studio/dots-3-note-preview:free": 32_000,
  "google/gemma-4-26b-a4b-it:free": 200_000,
  "google/gemma-4-31b-it:free": 200_000,
  "inclusionai/ling-3.0-flash-fin:free": 128_000,
  "liquid/lfm-2.5-2.6b:free": 32_000,
  "minimax/minimax-m2.7:free": 200_000,
  "minimax/minimax-m3:free": 200_000,
  "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free": 200_000,
  "nvidia/nemotron-3-super-120b-a12b:free": 200_000,
  "nvidia/nemotron-3.5-content-safety:free": 200_000,
  "nvidia/nemotron-3.5-lightning:free": 1_000_000,
  "nvidia/nemotron-3-ultra-550b-a55b:free": 1_000_000,
  "poolside/laguna-s-2.1:free": 200_000,
  "poolside/laguna-xs-2.1:free": 200_000,
  "thinkingmachines/inkling-small:free": 32_000,
  "thinkingmachines/inkling:free": 200_000,
  "z-ai/glm-5.2:free": 200_000,
};

/**
 * Per-model image-input capability. Verified on minimax/minimax-m3:free.
 * Default for unlisted IDs is false; if a model is known to support images
 * and isn't in the table, add it here.
 */
export const CLINE_SUPPORTS_IMAGES: Record<string, boolean> = {
  "minimax/minimax-m3:free": true,
};

/** Default context window for free IDs not in CLINE_CONTEXT_WINDOWS. */
export const CLINE_CONTEXT_WINDOW_DEFAULT = 200_000;