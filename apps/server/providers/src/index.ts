/** Antigravity provider — OAuth, daily-cloudcode-pa endpoint, session envelope */
export { createAntigravityStreamFn } from "./antigravity/index.js";
export type { AntigravitySessionState } from "./antigravity/index.js";

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

/** Cline provider — OpenAI-compatible, free tier, Bearer auth */
export {
  clineStreamFn,
  fetchClineFreeModels,
  isClineFreeModelId,
  getClineContextWindow,
  getClineSupportsImages,
  CLINE_BASE_URL,
  CLINE_FREE_MODEL_IDS,
  CLINE_CONTEXT_WINDOWS,
  CLINE_CONTEXT_WINDOW_DEFAULT,
  CLINE_SUPPORTS_IMAGES,
  loadClineCredential,
  saveClineCredential,
  clearClineCredential,
} from "./cline/index.js";
export type { ClineCredential } from "./cline/index.js";

/** Model discovery */
export { fetchAvailableModels } from "./discovery/fetch-models.js";
export type { FetchAvailableModelsOptions, DiscoveredApiModel } from "./discovery/fetch-models.js";

/** Auth utilities (useful for pre-loading or refreshing tokens) */
export { loadCredential, saveCredential, parseCredential } from "./auth/token-store.js";
export { refreshIfNeeded } from "./auth/token-refresh.js";

/** OAuth login flows */
export { loginAntigravity } from "./auth/login.js";

/** Types */
export type { ParsedCredential, GeminiOAuthCredential } from "./types/index.js";
