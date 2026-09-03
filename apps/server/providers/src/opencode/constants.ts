/**
 * OpenCode Zen provider — OpenAI-compatible chat completions endpoint.
 *
 * Free-tier models require no API key. Base URL is hardcoded.
 * Endpoint: https://opencode.ai/zen/v1/chat/completions
 * Model discovery: https://opencode.ai/zen/v1/models (filtered to free ids)
 */
export const OPENCODE_BASE_URL = "https://opencode.ai/zen/v1";
export const OPENCODE_USER_AGENT = "opencode/1.18.27";

/** Models confirmed free on the OpenCode Zen tier. Most free models use the
 *  `-free` id suffix and are auto-discovered by `isOpencodeFreeModelId`.
 *  This list only needs to cover free models that DON'T follow that convention
 *  (currently just "big-pickle") — it also serves as the offline fallback. */
export const OPENCODE_FREE_MODEL_IDS = [
  "big-pickle",
] as const;

export const OPENCODE_CONTEXT_WINDOW = 200_000;
