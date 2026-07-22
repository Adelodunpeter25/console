// Gemini CLI Constants
export const GEMINI_CLI_CLIENT_ID =
  process.env.GEMINI_CLI_CLIENT_ID ??
  Buffer.from(
    "NjgxMjU1ODA5Mzk1LW9vOGZ0Mm9wcmRybnA5ZTNhcWY2YXYzaG1kaWIxMzVqLmFwcHMuZ29vZ2xldXNlcmNvbnRlbnQuY29t",
    "base64",
  ).toString("utf-8");

export const GEMINI_CLI_CLIENT_SECRET =
  process.env.GEMINI_CLI_CLIENT_SECRET ??
  Buffer.from("R09DU1BYLTR1SGdNUG0tMW83U2stZ2VWNkN1NWNsWEZzeGw=", "base64").toString("utf-8");

export const GEMINI_BASE_URL = "https://cloudcode-pa.googleapis.com";
export const DEFAULT_GEMINI_CLI_VERSION = "0.46.0";

// Antigravity Constants
export const ANTIGRAVITY_CLIENT_ID =
  process.env.ANTIGRAVITY_CLIENT_ID ??
  Buffer.from(
    "MTA3MTAwNjA2MDU5MS10bWhzc2luMmgyMWxjcmUyMzV2dG9sb2poNGc0MDNlcC5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbQ==",
    "base64",
  ).toString("utf-8");

export const ANTIGRAVITY_CLIENT_SECRET =
  process.env.ANTIGRAVITY_CLIENT_SECRET ??
  Buffer.from("R09DU1BYLUs1OEZXUjQ4NkxkTEoxbUxCOHNYQzR6NnFEQWY=", "base64").toString("utf-8");

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

// ---------------------------------------------------------------------------
// User-Agent & Header Helpers
// ---------------------------------------------------------------------------

export function getAntigravityUserAgent(): string {
  const version = process.env.ANTIGRAVITY_VERSION ?? DEFAULT_ANTIGRAVITY_VERSION;
  const os = process.platform === "win32" ? "windows" : process.platform;
  const arch = process.arch === "x64" ? "amd64" : process.arch === "ia32" ? "386" : process.arch;
  return `antigravity/hub/${version} ${os}/${arch}`;
}

export function getGeminiCliUserAgent(modelId = "gemini-2.5-pro"): string {
  const version = process.env.GEMINI_CLI_VERSION ?? DEFAULT_GEMINI_CLI_VERSION;
  const platform = process.platform === "win32" ? "win32" : process.platform;
  const arch = process.arch === "x64" ? "x64" : process.arch;
  return `GeminiCLI/${version}/${modelId} (${platform}; ${arch}; terminal)`;
}

export function getGeminiCliHeaders(modelId?: string): Record<string, string> {
  return {
    "User-Agent": getGeminiCliUserAgent(modelId),
    "Client-Metadata": "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI",
  };
}
