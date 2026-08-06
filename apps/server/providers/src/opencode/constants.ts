/**
 * OpenCode Zen provider — OpenAI-compatible chat completions endpoint.
 *
 * Free-tier models require no API key. Base URL is hardcoded.
 * Endpoint: https://opencode.ai/zen/v1/chat/completions
 * Model discovery: https://opencode.ai/zen/v1/models (filtered to free ids)
 */
export const OPENCODE_BASE_URL = "https://opencode.ai/zen/v1";

/** Models confirmed free on the OpenCode Zen tier. */
export const OPENCODE_FREE_MODEL_IDS = [
  "big-pickle",
  "deepseek-v4-flash-free",
  "mimo-v2.5-free",
  "ling-3.0-flash-free",
  "nemotron-3-ultra-free",
  "north-mini-code-free",
  "laguna-s-2.1-free",
  "longcat-2.0-free",
] as const;

export const OPENCODE_CONTEXT_WINDOW = 200_000;
