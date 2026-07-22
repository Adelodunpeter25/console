/** Antigravity provider — OAuth, daily-cloudcode-pa endpoint, session envelope */
export { createAntigravityStreamFn } from "./antigravity/index.js";
export type { AntigravitySessionState } from "./antigravity/index.js";

/** Gemini CLI provider — OAuth, cloudcode-pa endpoint */
export { geminiStreamFn } from "./gemini/index.js";

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
