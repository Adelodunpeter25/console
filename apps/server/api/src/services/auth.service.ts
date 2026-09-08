/**
 * Remote OAuth Auth Service.
 */
import {
  ANTIGRAVITY_OAUTH_CONFIG,
  OAUTH_AUTH_URL,
} from "@/providers/src/constants.js";
import { loadCredential } from "@/providers/src/auth/token-store.js";
import { completeAuthFlowWithCode } from "@/providers/src/auth/login.js";
import {
  getConfiguredProjectId,
  setConfiguredProjectId,
} from "@/providers/src/auth/provider-config.js";
import {
  codexCredentialExists,
  createCodexAuthorizationUrl,
  exchangeCodexCode,
  generateCodexPkce,
  loadCodexCredential,
  saveCodexCredential,
  createDevinAuthorizationUrl,
  devinCredentialExists,
  exchangeDevinCode,
  generateDevinPkce,
  saveDevinCredential,
  DEVIN_CALLBACK_PATH,
  DEVIN_CALLBACK_PORT,
} from "@/providers/src/index.js";
import * as crypto from "node:crypto";
import type { AuthStatusResponse } from "@/api/src/types/index.js";
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
  private readonly codexPending = new Map<string, { verifier: string; expiresAt: number }>();
  /** Pending state tokens for antigravity loopback OAuth flows. */
  private readonly oauthPending = new Map<string, { provider: OAuthProviderId; expiresAt: number }>();
  /** Pending state tokens for Devin PKCE OAuth flows. */
  private readonly devinPending = new Map<string, { verifier: string; expiresAt: number }>();

  async getAuthStatus(): Promise<AuthStatusResponse> {
    const antigravityCred = await tryLoadCredential("antigravity");
    const codexCred = await (async () => {
      try {
        return await loadCodexCredential();
      } catch {
        return null;
      }
    })();
    const devinLoggedIn = await devinCredentialExists();

    const antigravityConfigured = await getConfiguredProjectId("antigravity");

    return {
      antigravity: {
        loggedIn: Boolean(antigravityCred?.accessToken),
        email: antigravityCred?.email,
        projectId: antigravityCred?.projectId,
        configuredProjectId: antigravityConfigured,
      },
      codex: {
        loggedIn: Boolean(codexCred?.accessToken) || (await codexCredentialExists()),
        email: codexCred?.email,
      },
      devin: {
        loggedIn: devinLoggedIn,
      },
    };
  }

  getLoginUrl(provider: OAuthProviderId): {
    provider: string;
    authUrl: string;
    state: string;
    redirectUri: string;
  } {
    if (provider === "codex") {
      const state = crypto.randomBytes(24).toString("hex");
      const { verifier, challenge } = generateCodexPkce();
      const result = createCodexAuthorizationUrl({ state, verifierChallenge: challenge });
      this.codexPending.set(state, { verifier, expiresAt: Date.now() + 10 * 60_000 });
      return { provider, authUrl: result.authUrl, state, redirectUri: result.redirectUri };
    }

    if (provider === "devin") {
      const state = crypto.randomBytes(24).toString("hex");
      const { verifier, challenge } = generateDevinPkce();
      const result = createDevinAuthorizationUrl({ state, verifierChallenge: challenge });
      this.devinPending.set(state, { verifier, expiresAt: Date.now() + 10 * 60_000 });
      return { provider, authUrl: result.authUrl, state, redirectUri: result.redirectUri };
    }

    const oauthConfig = ANTIGRAVITY_OAUTH_CONFIG;

    const state = crypto.randomBytes(24).toString("hex");
    this.oauthPending.set(state, { provider, expiresAt: Date.now() + 10 * 60_000 });

    const redirectUri = `http://localhost:${oauthConfig.port}${oauthConfig.callbackPath}`;
    const scopeString = oauthConfig.scopes.join(" ");

    const authUrl = `${OAUTH_AUTH_URL}?response_type=code&client_id=${encodeURIComponent(
      oauthConfig.clientId,
    )}&redirect_uri=${encodeURIComponent(redirectUri)}&scope=${encodeURIComponent(
      scopeString,
    )}&access_type=offline&prompt=consent&state=${encodeURIComponent(state)}`;

    return {
      provider,
      authUrl,
      state,
      redirectUri,
    };
  }

  async handleCallback(
    provider: OAuthProviderId,
    code: string,
    state?: string,
  ): Promise<{ provider: string; userEmail?: string; projectId?: string }> {
    if (provider === "codex") {
      if (!state) throw new Error("Codex OAuth callback is missing state.");
      const pending = this.codexPending.get(state);
      this.codexPending.delete(state);
      if (!pending || pending.expiresAt < Date.now()) throw new Error("Codex OAuth state is invalid or expired.");
      const credential = await exchangeCodexCode(code, pending.verifier, "http://localhost:1455/auth/callback");
      await saveCodexCredential(credential);
      return { provider, userEmail: credential.email };
    }

    if (provider === "devin") {
      if (!state) throw new Error("Devin OAuth callback is missing state.");
      const pending = this.devinPending.get(state);
      this.devinPending.delete(state);
      if (!pending || pending.expiresAt < Date.now()) throw new Error("Devin OAuth state is invalid or expired.");
      const redirectUri = `http://127.0.0.1:${DEVIN_CALLBACK_PORT}${DEVIN_CALLBACK_PATH}`;
      const credential = await exchangeDevinCode(code, pending.verifier, redirectUri);
      await saveDevinCredential(credential);
      return { provider };
    }

    // Validate the state token for antigravity.
    if (state) {
      const pending = this.oauthPending.get(state);
      this.oauthPending.delete(state);
      if (!pending || pending.expiresAt < Date.now()) {
        throw new Error(`OAuth state is invalid or expired for provider: ${provider}`);
      }
    }

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
