/**
 * Claude (Anthropic subscription) backend constants.
 *
 * OAuth mirrors pi's `packages/ai/src/auth/oauth/anthropic.ts` (Claude
 * Pro/Max PKCE flow) and oh-my-pi's Claude Code fingerprint
 * (`packages/ai/src/providers/claude-code-fingerprint.ts`).
 */
export const CLAUDE_BASE_URL = process.env.CLAUDE_BASE_URL ?? "https://api.anthropic.com";
/** Claude Code CLI OAuth client ID (same value pi embeds base64-encoded). */
export const CLAUDE_CLIENT_ID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e";
export const CLAUDE_AUTHORIZE_URL = "https://claude.ai/oauth/authorize";
export const CLAUDE_TOKEN_URL = "https://platform.claude.com/v1/oauth/token";
export const CLAUDE_CALLBACK_PORT = 53692;
export const CLAUDE_CALLBACK_PATH = "/callback";
export const CLAUDE_SCOPE =
  "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload";
export const CLAUDE_REFRESH_SKEW_MS = 5 * 60_000;

/** Claude Code CLI version represented on the Anthropic wire. */
export const CLAUDE_CODE_VERSION = "2.1.257";
/** `@anthropic-ai/sdk` version bundled by the current Claude Code release. */
export const CLAUDE_SDK_VERSION = "0.112.1";
/** User-Agent emitted by Claude Code's CLI inference entrypoint. */
export const CLAUDE_USER_AGENT = `claude-cli/${CLAUDE_CODE_VERSION} (external, cli)`;

/** Betas sent on OAuth (subscription) requests, mirroring oh-my-pi. */
export const CLAUDE_OAUTH_BETAS = [
  "claude-code-20250219",
  "oauth-2025-04-20",
  "interleaved-thinking-2025-05-14",
];

/** Per-request output-token ceiling, mirroring Claude Code. */
export const CLAUDE_MAX_OUTPUT_TOKENS = 32_000;

/**
 * Extended-thinking token budget, always enabled. "Medium" level per pi's
 * `thinkingBudgetForLevel` (minimal 1024 / low 2048 / medium 8192 / high
 * 16384). Must stay below `CLAUDE_MAX_OUTPUT_TOKENS`.
 */
export const CLAUDE_THINKING_BUDGET_TOKENS = 8192;

export function claudeRedirectUri(): string {
  return `http://localhost:${CLAUDE_CALLBACK_PORT}${CLAUDE_CALLBACK_PATH}`;
}

export function claudeMessagesUrl(baseUrl = CLAUDE_BASE_URL): string {
  const normalized = baseUrl.replace(/\/+$/, "");
  // `?beta=true` marks Claude Code OAuth traffic, mirroring oh-my-pi.
  return `${normalized}/v1/messages?beta=true`;
}

export function claudeModelsUrl(baseUrl = CLAUDE_BASE_URL): string {
  return `${baseUrl.replace(/\/+$/, "")}/v1/models`;
}
