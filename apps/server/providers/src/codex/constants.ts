/** OpenAI Codex (ChatGPT subscription) backend constants. */
export const CODEX_BASE_URL = process.env.CODEX_BASE_URL ?? "https://chatgpt.com/backend-api";
export const CODEX_CLIENT_VERSION = "0.144.1";
export const CODEX_CLIENT_ID = "app_EMoamEEZ73f0CkXaXp7hrann";
export const CODEX_AUTHORIZE_URL = "https://auth.openai.com/oauth/authorize";
export const CODEX_TOKEN_URL = "https://auth.openai.com/oauth/token";
export const CODEX_CALLBACK_PORT = 1455;
export const CODEX_CALLBACK_PATH = "/auth/callback";
export const CODEX_SCOPE =
  "openid profile email offline_access api.connectors.read api.connectors.invoke";
export const CODEX_ACCOUNT_CLAIM = "https://api.openai.com/auth";
export const CODEX_PROFILE_CLAIM = "https://api.openai.com/profile";
export const CODEX_REFRESH_SKEW_MS = 60_000;

export function codexResponsesUrl(baseUrl = CODEX_BASE_URL): string {
  const normalized = baseUrl.replace(/\/+$/, "");
  return normalized.endsWith("/codex/responses") ? normalized : `${normalized}/codex/responses`;
}

export function codexModelsUrl(baseUrl = CODEX_BASE_URL): string {
  return `${baseUrl.replace(/\/+$/, "")}/codex/models?client_version=${encodeURIComponent(CODEX_CLIENT_VERSION)}`;
}
