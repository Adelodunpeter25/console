/** Antigravity provider — OAuth, daily-cloudcode-pa endpoint, session envelope */
export { createAntigravityStreamFn } from "./antigravity/index.js";
export type { AntigravitySessionState } from "./antigravity/index.js";

/** Gemini CLI provider — OAuth, cloudcode-pa endpoint */
export { geminiStreamFn } from "./gemini/index.js";

/** OpenCode Zen provider — free OpenAI-compatible endpoint */
export { opencodeStreamFn, fetchOpencodeFreeModels, OPENCODE_FREE_MODEL_IDS } from "./opencode/index.js";

/** OpenAI Codex provider — ChatGPT OAuth and Codex Responses API. */
export { codexStreamFn } from "./codex/stream-fn.js";
export {
  codexCredentialExists,
  createCodexAuthorizationUrl,
  exchangeCodexCode,
  generateCodexPkce,
  loadCodexCredential,
  refreshCodexIfNeeded,
  saveCodexCredential,
} from "./codex/oauth.js";
export type { CodexOAuthCredential, ParsedCodexCredential } from "./codex/oauth.js";

/** Codebuff / Freebuff provider — device-code login + OpenAI-compatible backend */
export {
  codebuffStreamFn,
  startCodebuffLogin,
  pollCodebuffLogin,
  generateFingerprintId,
  loadCodebuffCredential,
  clearCodebuffCredential,
  hasCodebuffCredential,
  CODEBUFF_BASE_URL,
  CODEBUFF_API_URL,
  CODEBUFF_MODEL_SPECS,
  isCodebuffFreeModelId,
} from "./codebuff/index.js";
export type {
  CodebuffCredential,
  CodebuffLoginCode,
  CodebuffLoginStatus,
} from "./codebuff/index.js";

/** Model discovery */
export { fetchAvailableModels } from "./discovery/fetch-models.js";
export type { FetchAvailableModelsOptions, DiscoveredApiModel } from "./discovery/fetch-models.js";

/** Auth utilities (useful for pre-loading or refreshing tokens) */
export { loadCredential, saveCredential, parseCredential } from "./auth/token-store.js";
export { refreshIfNeeded } from "./auth/token-refresh.js";

/** OAuth login flows */
export { loginGemini, loginAntigravity } from "./auth/login.js";

/** Types */
export type { ParsedCredential, GeminiOAuthCredential } from "./types/index.js";
