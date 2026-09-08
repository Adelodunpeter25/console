/**
 * Devin (Codeium/Windsurf) provider constants.
 *
 * Two distinct hosts:
 * - `app.devin.ai` / `api.devin.ai` — OAuth login + CLI token exchange.
 * - `server.codeium.com` — Connect/gRPC streaming + JWT auth (see stream-fn.ts).
 */
export const DEVIN_WEBAPP_URL = "https://app.devin.ai";
export const DEVIN_API_URL = "https://api.devin.ai";
export const DEVIN_OAUTH_AUTHORIZE_URL = `${DEVIN_WEBAPP_URL}/auth/cli/continue`;
export const DEVIN_TOKEN_URL = `${DEVIN_API_URL}/auth/cli/token`;
export const DEVIN_CALLBACK_PORT = 59653;
export const DEVIN_CALLBACK_PATH = "/callback";

/**
 * Long-lived fallback when the CLI token isn't a JWT (some non-expiring tokens
 * are opaque). One year is conservative — the user can re-login by then.
 */
export const DEVIN_FALLBACK_EXPIRES_MS = 365 * 24 * 60 * 60 * 1000;
