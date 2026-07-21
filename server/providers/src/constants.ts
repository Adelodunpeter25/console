// Gemini CLI Constants
export const GEMINI_CLI_CLIENT_ID =
  "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com";
export const GEMINI_CLI_CLIENT_SECRET = "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";
export const GEMINI_BASE_URL = "https://cloudcode-pa.googleapis.com";
export const DEFAULT_GEMINI_CLI_VERSION = "0.46.0";

// Antigravity Constants
export const ANTIGRAVITY_CLIENT_ID =
  "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com";
export const ANTIGRAVITY_CLIENT_SECRET = "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";
export const ANTIGRAVITY_BASE_URL = "https://daily-cloudcode-pa.googleapis.com";
export const DEFAULT_ANTIGRAVITY_VERSION = "2.1.4";

// OAuth Constants
export const OAUTH_AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth";
export const OAUTH_TOKEN_URL = "https://oauth2.googleapis.com/token";
export const USERINFO_URL = "https://www.googleapis.com/oauth2/v1/userinfo";
export const REFRESH_SKEW_MS = 60_000;

// Scope Definitions
const GEMINI_SCOPES = [
  "https://www.googleapis.com/auth/cloud-platform",
  "https://www.googleapis.com/auth/userinfo.email",
  "https://www.googleapis.com/auth/userinfo.profile",
];

const ANTIGRAVITY_SCOPES = [
  ...GEMINI_SCOPES,
  "https://www.googleapis.com/auth/cclog",
  "https://www.googleapis.com/auth/experimentsandconfigs",
];

// Full OAuth Configs
export const GEMINI_OAUTH_CONFIG = {
  type: "gemini" as const,
  port: 8085,
  callbackPath: "/oauth2callback",
  scopes: GEMINI_SCOPES,
  clientId: GEMINI_CLI_CLIENT_ID,
  clientSecret: GEMINI_CLI_CLIENT_SECRET,
  ideType: "GEMINI_CLI" as const,
};

export const ANTIGRAVITY_OAUTH_CONFIG = {
  type: "antigravity" as const,
  port: 51121,
  callbackPath: "/oauth-callback",
  scopes: ANTIGRAVITY_SCOPES,
  clientId: ANTIGRAVITY_CLIENT_ID,
  clientSecret: ANTIGRAVITY_CLIENT_SECRET,
  ideType: "ANTIGRAVITY" as const,
};
