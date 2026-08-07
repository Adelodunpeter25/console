/**
 * Codebuff / Freebuff provider constants.
 *
 * The Codebuff backend is an OpenAI-compatible chat-completions API:
 *   POST https://codebuff.com/api/v1/chat/completions
 *   Authorization: Bearer <authToken>
 *
 * The authToken is obtained through the device-code style login flow
 * (same endpoints the official CLI uses) and stored in
 * ~/.console/codebuff-creds.json, or provided via the CODEBUFF_API_KEY
 * environment variable.
 */

/** Base host for the Codebuff backend. Overridable for tests/proxies.
 *  The canonical API host is www.codebuff.com (bare codebuff.com 301-redirects). */
export const CODEBUFF_BASE_URL =
  process.env.CODEBUFF_BASE_URL ?? "https://www.codebuff.com";

/** OpenAI-compatible API root (chat completions live at ${this}/chat/completions). */
export const CODEBUFF_API_URL = `${CODEBUFF_BASE_URL}/api/v1`;

/** Env var that can supply an auth token (matches the official CLI). */
export const CODEBUFF_API_KEY_ENV_VAR = "CODEBUFF_API_KEY";

/** Credential file name under ~/.console. */
export const CODEBUFF_CREDENTIALS_FILE = "codebuff-creds.json";

/**
 * Free-tier model catalog (mirrors Freebuff's SUPPORTED_FREEBUFF_MODELS).
 *
 * `premium` models draw from the shared daily premium session pool (capped
 * server-side); `premium: false` models (DeepSeek V4 Flash, MiMo 2.5) are
 * always available. Context windows come from the Freebuff agent runtime
 * (Kimi 250k, everything else 400k).
 */
export const CODEBUFF_FREE_MODEL_IDS = [
  "deepseek/deepseek-v4-flash",
  "mimo/mimo-v2.5",
] as const;

export interface CodebuffModelSpec {
  id: string;
  premium: boolean;
  multimodal: boolean;
  contextWindow: number;
}

export const CODEBUFF_MODEL_SPECS: readonly CodebuffModelSpec[] = [
  { id: "deepseek/deepseek-v4-flash", premium: false, multimodal: false, contextWindow: 400_000 },
  { id: "mimo/mimo-v2.5", premium: false, multimodal: true, contextWindow: 400_000 },
  { id: "minimax/minimax-m3", premium: true, multimodal: true, contextWindow: 400_000 },
  { id: "deepseek/deepseek-v4-pro", premium: true, multimodal: false, contextWindow: 400_000 },
  { id: "moonshotai/kimi-k2.7-code", premium: true, multimodal: true, contextWindow: 250_000 },
  { id: "mimo/mimo-v2.5-pro", premium: true, multimodal: false, contextWindow: 400_000 },
] as const;

export function isCodebuffFreeModelId(id: string): boolean {
  return (CODEBUFF_FREE_MODEL_IDS as readonly string[]).includes(id);
}
