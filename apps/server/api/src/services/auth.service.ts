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
import {
  getConfiguredProjectId,
  setConfiguredProjectId,
} from "../../../providers/src/auth/provider-config.js";
import {
  hasCodebuffCredential,
  loadCodebuffCredential,
  pollCodebuffLogin,
  startCodebuffLogin,
} from "../../../providers/src/index.js";
import type { AuthStatusResponse } from "../types/index.js";
import type { OAuthProviderId } from "@console/types";

/**
 * Safely attempt to load a credential, returning null when the user is not
 * logged in (no credential file, invalid JSON, missing token, etc.) instead of
 * throwing — so getAuthStatus() can report "not logged in" gracefully.
 */
async function tryLoadCredential(type: OAuthProviderId) {
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
    const codebuffCred = await loadCodebuffCredential();

    const [geminiConfigured, antigravityConfigured] = await Promise.all([
      getConfiguredProjectId("gemini"),
      getConfiguredProjectId("antigravity"),
    ]);

    return {
      gemini: {
        loggedIn: Boolean(geminiCred?.accessToken),
        email: geminiCred?.email,
        projectId: geminiCred?.projectId,
        configuredProjectId: geminiConfigured,
      },
      antigravity: {
        loggedIn: Boolean(antigravityCred?.accessToken),
        email: antigravityCred?.email,
        projectId: antigravityCred?.projectId,
        configuredProjectId: antigravityConfigured,
      },
      codebuff: {
        loggedIn: Boolean(codebuffCred?.authToken),
        email: codebuffCred?.email,
      },
    };
  }

  /**
   * Start the Codebuff device-code login flow. Returns the login URL the user
   * must open plus the params needed to poll for completion.
   */
  async startCodebuffLogin() {
    const login = await startCodebuffLogin();
    return { provider: "codebuff", ...login };
  }

  /**
   * Poll the Codebuff login status. Once the user completes the browser login
   * the credential is stored server-side and loggedIn flips to true.
   */
  async pollCodebuffLogin(params: {
    fingerprintId: string;
    fingerprintHash: string;
    expiresAt: string;
  }) {
    return pollCodebuffLogin(params);
  }

  /** True when a Codebuff credential (env var or file) is present. */
  async hasCodebuffLogin(): Promise<boolean> {
    return hasCodebuffCredential();
  }

  getLoginUrl(provider: OAuthProviderId): {
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
    provider: OAuthProviderId,
    code: string,
  ): Promise<{ provider: string; userEmail?: string; projectId?: string }> {
    // Load the user-configured project ID (if any) so it takes precedence
    // over env vars during loadCodeAssist.
    const configuredProjectId = await getConfiguredProjectId(provider);
    const cred = await completeAuthFlowWithCode(provider, code, configuredProjectId);
    return {
      provider,
      userEmail: cred.email,
      projectId: cred.projectId,
    };
  }

  async getProjectId(provider: OAuthProviderId): Promise<string | undefined> {
    return getConfiguredProjectId(provider);
  }

  async setProjectId(
    provider: OAuthProviderId,
    projectId: string | undefined,
  ): Promise<void> {
    await setConfiguredProjectId(provider, projectId);
  }
}
