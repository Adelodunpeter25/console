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

const ANTIGRAVITY_SCOPES = [
  "https://www.googleapis.com/auth/cloud-platform",
  "https://www.googleapis.com/auth/userinfo.email",
  "https://www.googleapis.com/auth/userinfo.profile",
  "https://www.googleapis.com/auth/cclog",
  "https://www.googleapis.com/auth/experimentsandconfigs",
];

// Full OAuth Config
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
// User-Agent Helper
// ---------------------------------------------------------------------------

export function getAntigravityUserAgent(): string {
  const version = process.env.ANTIGRAVITY_VERSION ?? DEFAULT_ANTIGRAVITY_VERSION;
  const os = process.platform === "win32" ? "windows" : process.platform;
  const arch = process.arch === "x64" ? "amd64" : process.arch === "ia32" ? "386" : process.arch;
  return `antigravity/hub/${version} ${os}/${arch}`;
}
