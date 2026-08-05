/**
 * Remote OAuth Auth Service.
 */
import {
  ANTIGRAVITY_OAUTH_CONFIG,
  GEMINI_OAUTH_CONFIG,
  OAUTH_AUTH_URL,
} from "../../../providers/src/constants.js";
import { loadCredential } from "../../../providers/src/auth/token-store.js";
import { completeAuthFlowWithCode } from "../../../providers/src/auth/login.js";
import type { AuthStatusResponse } from "../types/index.js";

/**
 * Safely attempt to load a credential, returning null when the user is not
 * logged in (no credential file, invalid JSON, missing token, etc.) instead of
 * throwing — so getAuthStatus() can report "not logged in" gracefully.
 */
async function tryLoadCredential(type: "gemini" | "antigravity") {
  try {
    return await loadCredential(type);
  } catch {
    return null;
  }
}

export class AuthService {
  async getAuthStatus(): Promise<AuthStatusResponse> {
    const geminiCred = await tryLoadCredential("gemini");
    const antigravityCred = await tryLoadCredential("antigravity");

    return {
      gemini: {
        loggedIn: Boolean(geminiCred?.accessToken),
        email: geminiCred?.email,
        projectId: geminiCred?.projectId,
      },
      antigravity: {
        loggedIn: Boolean(antigravityCred?.accessToken),
        email: antigravityCred?.email,
        projectId: antigravityCred?.projectId,
      },
    };
  }

  getLoginUrl(provider: "gemini" | "antigravity"): {
    provider: string;
    authUrl: string;
    redirectUri: string;
  } {
    const oauthConfig = provider === "gemini" ? GEMINI_OAUTH_CONFIG : ANTIGRAVITY_OAUTH_CONFIG;

    const redirectUri = `http://localhost:${oauthConfig.port}${oauthConfig.callbackPath}`;
    const scopeString = oauthConfig.scopes.join(" ");

    const authUrl = `${OAUTH_AUTH_URL}?response_type=code&client_id=${encodeURIComponent(
      oauthConfig.clientId,
    )}&redirect_uri=${encodeURIComponent(redirectUri)}&scope=${encodeURIComponent(
      scopeString,
    )}&access_type=offline&prompt=consent`;

    return {
      provider,
      authUrl,
      redirectUri,
    };
  }

  async handleCallback(
    provider: "gemini" | "antigravity",
    code: string,
  ): Promise<{ provider: string; userEmail?: string; projectId?: string }> {
    const cred = await completeAuthFlowWithCode(provider, code);
    return {
      provider,
      userEmail: cred.email,
      projectId: cred.projectId,
    };
  }
}
