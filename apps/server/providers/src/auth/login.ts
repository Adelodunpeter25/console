/**
 * OAuth login flow implementation for Antigravity.
 *
 * - Spins up local callback server
 * - Opens browser for auth
 * - Exchanges code for tokens
 * - Calls loadCodeAssist / onboardUser to get projectId
 * - Saves credential to ~/.console/antigravity-creds.json
 */

import * as crypto from "node:crypto";
import { exec } from "node:child_process";
import { promisify } from "node:util";
import type { GeminiOAuthCredential } from "@/providers/src/types/index.js";
import type { OAuthProviderId } from "@console/types";
import { saveCredential } from "./token-store.js";
import {
  OAUTH_AUTH_URL,
  OAUTH_TOKEN_URL,
  USERINFO_URL,
  ANTIGRAVITY_OAUTH_CONFIG,
  getAntigravityUserAgent,
} from "@/providers/src/constants.js";

const execAsync = promisify(exec);

function getDefaultTier(allowedTiers?: Array<{ id?: string; isDefault?: boolean }>): {
  id?: string;
} {
  const TIER_LEGACY = "legacy-tier";
  if (!allowedTiers || allowedTiers.length === 0) return { id: TIER_LEGACY };
  const defaultTier = allowedTiers.find((t) => t.isDefault);
  return defaultTier ?? { id: TIER_LEGACY };
}

const TIER_FREE = "free-tier";
const TIER_STANDARD = "standard-tier";

interface GoogleRpcErrorResponse {
  error?: {
    details?: Array<{ reason?: string }>;
  };
}

function isVpcScAffectedUser(payload: unknown): boolean {
  if (!payload || typeof payload !== "object") return false;
  if (!("error" in payload)) return false;
  const error = (payload as GoogleRpcErrorResponse).error;
  if (!error?.details || !Array.isArray(error.details)) return false;
  return error.details.some((detail) => detail.reason === "SECURITY_POLICY_VIOLATED");
}

function readProjectId(value: string | { id?: string } | undefined): string | undefined {
  if (typeof value === "string" && value.length > 0) {
    return value;
  }
  if (value && typeof value === "object" && typeof value.id === "string" && value.id.length > 0) {
    return value.id;
  }
  return undefined;
}

async function reportDebugEvent(
  hypothesisId: string,
  location: string,
  msg: string,
  data: Record<string, unknown>,
): Promise<void> {
  try {
    const envText = await import("node:fs/promises").then((fs) =>
      fs.readFile(
        "/Users/techclub/Documents/projects/console/.dbg/antigravity-onboard-poll.env",
        "utf8",
      ),
    );
    const serverUrl =
      envText.match(/DEBUG_SERVER_URL=(.+)/)?.[1]?.trim() ?? "http://127.0.0.1:7777/event";
    const sessionId =
      envText.match(/DEBUG_SESSION_ID=(.+)/)?.[1]?.trim() ?? "antigravity-onboard-poll";
    await fetch(serverUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        sessionId,
        runId: "pre-fix",
        hypothesisId,
        location,
        msg,
        data,
        ts: Date.now(),
      }),
    }).catch(() => {});
  } catch {
    // Best-effort debug reporting only.
  }
}

/**
 * Opens browser for a given URL — works on macOS, Linux, Windows
 */
async function openBrowser(url: string): Promise<void> {
  const command =
    process.platform === "darwin"
      ? `open "${url}"`
      : process.platform === "win32"
        ? `start "" "${url}"`
        : `xdg-open "${url}"`;
  await execAsync(command);
}

/**
 * Spins up a local server to receive the OAuth callback
 */
function startCallbackServer(
  port: number,
  callbackPath: string,
): Promise<{ code: string; state: string; close: () => void }> {
  return new Promise((resolve, reject) => {
    let settled = false;

    try {
      const server = Bun.serve({
        port,
        async fetch(req) {
          const url = new URL(req.url);
          if (!url.pathname.startsWith(callbackPath)) {
            return new Response("Not found", { status: 404 });
          }
          const code = url.searchParams.get("code");
          const state = url.searchParams.get("state");

          if (code && state && !settled) {
            settled = true;
            resolve({ code, state, close: () => server.stop(true) });
          } else if (!code || !state) {
            settled = true;
            reject(new Error("Invalid callback: missing code or state"));
          }

          return new Response(
            `
          <!DOCTYPE html>
          <html>
          <head><title>Login Successful</title></head>
          <body>
            <h1>Successfully authenticated!</h1>
            <p>You can close this tab now.</p>
          </body>
          </html>
        `,
            { headers: { "Content-Type": "text/html" } },
          );
        },
      });

      console.log(`Callback server listening on http://localhost:${port}`);
      void server;
    } catch (error) {
      reject(error instanceof Error ? error : new Error(String(error)));
    }
  });
}

/**
 * Exchanges authorization code for tokens
 */
async function exchangeCodeForTokens(
  code: string,
  redirectUri: string,
): Promise<{ access_token: string; refresh_token: string; expires_in: number }> {
  const body = new URLSearchParams({
    client_id: ANTIGRAVITY_OAUTH_CONFIG.clientId,
    client_secret: ANTIGRAVITY_OAUTH_CONFIG.clientSecret,
    code,
    redirect_uri: redirectUri,
    grant_type: "authorization_code",
  });

  const response = await fetch(OAUTH_TOKEN_URL, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(`Token exchange failed (${response.status}): ${detail}`);
  }

  const data = (await response.json()) as {
    access_token: string;
    refresh_token: string;
    expires_in: number;
    token_type: string;
    scope: string;
    id_token?: string;
  };

  return {
    access_token: data.access_token,
    refresh_token: data.refresh_token,
    expires_in: data.expires_in,
  };
}

/**
 * Gets user email from userinfo endpoint
 */
async function getUserEmail(accessToken: string): Promise<string> {
  const response = await fetch(USERINFO_URL, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });

  if (!response.ok) {
    throw new Error("Failed to fetch user info");
  }

  const data = (await response.json()) as { email: string };
  return data.email;
}

/**
 * Calls loadCodeAssist API to get or provision project
 */
async function loadCodeAssist(
  accessToken: string,
  explicitProjectId?: string,
): Promise<string> {
  // Precedence: explicit (from UI/config) > env var
  const envProjectId = explicitProjectId || process.env.GOOGLE_CLOUD_PROJECT || process.env.GOOGLE_CLOUD_PROJECT_ID;
  const endpoint = `https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist`;

  const headers: Record<string, string> = {
    Authorization: `Bearer ${accessToken}`,
    "Content-Type": "application/json",
    "User-Agent": getAntigravityUserAgent(),
  };

  const response = await fetch(endpoint, {
    method: "POST",
    headers,
    body: JSON.stringify({
      metadata: {
        ideType: "ANTIGRAVITY",
        platform: "PLATFORM_UNSPECIFIED",
        pluginType: "GEMINI",
      },
    }),
  });

  // #region debug-point B:load-code-assist-response
  await reportDebugEvent(
    "B",
    "login.ts:loadCodeAssist",
    "[DEBUG] loadCodeAssist response received",
    {
      status: response.status,
      ok: response.ok,
      contentType: response.headers.get("content-type"),
    },
  );
  // #endregion

  let data: {
    cloudaicompanionProject?: string | { id?: string };
    currentTier?: { id?: string };
    allowedTiers?: Array<{ id?: string; isDefault?: boolean }>;
    name?: string;
    done?: boolean;
  };

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(`loadCodeAssist failed (${response.status}): ${detail}`);
  } else {
    data = (await response.json()) as {
      cloudaicompanionProject?: string | { id?: string };
      currentTier?: { id?: string };
      allowedTiers?: Array<{ id?: string; isDefault?: boolean }>;
      name?: string;
      done?: boolean;
    };
  }

  // #region debug-point C:load-code-assist-payload
  await reportDebugEvent("C", "login.ts:loadCodeAssist", "[DEBUG] loadCodeAssist payload parsed", {
    hasCurrentTier: Boolean(data.currentTier),
    allowedTierIds: data.allowedTiers?.map((tier) => tier.id).filter(Boolean) ?? [],
    defaultTierId: getDefaultTier(data.allowedTiers)?.id,
    projectValueType: typeof data.cloudaicompanionProject,
    hasProjectIdObject:
      typeof data.cloudaicompanionProject === "object" && Boolean(data.cloudaicompanionProject?.id),
  });
  // #endregion

  const existingProject = readProjectId(data.cloudaicompanionProject);
  if (existingProject) {
    return existingProject;
  }

  const tierId = getDefaultTier(data.allowedTiers)?.id ?? TIER_FREE;

  // If the account has a current tier but no project, check if it's a paid tier
  // For paid tiers, we need GOOGLE_CLOUD_PROJECT. For free tier, we can provision.
  if (data.currentTier) {
    const currentTierId = data.currentTier.id;
    // If it's a paid tier and no env project, throw error
    if (
      currentTierId &&
      currentTierId !== TIER_FREE &&
      currentTierId !== "legacy-tier" &&
      !envProjectId
    ) {
      throw new Error(
        "This account requires setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable.",
      );
    }
    // For free tier or legacy tier, proceed to provision
  }

  return onboardUser(accessToken, tierId, envProjectId);
}

/**
 * Calls onboardUser API to provision a project (polling until done)
 */
async function onboardUser(
  accessToken: string,
  tierId?: string,
  envProjectId?: string,
): Promise<string> {
  const endpoint = `https://daily-cloudcode-pa.googleapis.com/v1internal:onboardUser`;

  const headers: Record<string, string> = {
    Authorization: `Bearer ${accessToken}`,
    "Content-Type": "application/json",
    "User-Agent": getAntigravityUserAgent(),
  };

  const body: Record<string, unknown> = {
    metadata: {
      ideType: "ANTIGRAVITY",
      platform: "PLATFORM_UNSPECIFIED",
      pluginType: "GEMINI",
    },
  };

  if (tierId) {
    body.tierId = tierId;
  }

  const response = await fetch(endpoint, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });

  // #region debug-point A:onboard-user-response
  await reportDebugEvent("A", "login.ts:onboardUser", "[DEBUG] onboardUser response received", {
    tierId,
    status: response.status,
    ok: response.ok,
    contentType: response.headers.get("content-type"),
  });
  // #endregion

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(`onboardUser failed (${response.status}): ${detail}`);
  }

  // Poll for LRO completion
  const operation = (await response.json()) as {
    name?: string;
    done?: boolean;
    response?: { cloudaicompanionProject?: string | { id?: string } };
  };

  // #region debug-point A:onboard-user-payload
  await reportDebugEvent("A", "login.ts:onboardUser", "[DEBUG] onboardUser payload parsed", {
    tierId,
    done: operation.done,
    name: operation.name,
    hasResponse: Boolean(operation.response),
    projectValueType: typeof operation.response?.cloudaicompanionProject,
  });
  // #endregion

  const projectId = readProjectId(operation.response?.cloudaicompanionProject);
  if (operation.done && projectId) {
    return projectId;
  }

  if (!operation.done && operation.name) {
    for (let i = 0; i < 30; i++) {
      await new Promise((resolve) => setTimeout(resolve, 1000));
      const pollEndpoint = `https://daily-cloudcode-pa.googleapis.com/v1internal/${operation.name}`;
      const pollResponse = await fetch(pollEndpoint, {
        headers,
      });
      if (!pollResponse.ok) {
        const detail = await pollResponse.text().catch(() => "");
        throw new Error(`onboardUser poll failed (${pollResponse.status}): ${detail}`);
      }
      const pollData = (await pollResponse.json()) as {
        done?: boolean;
        response?: { cloudaicompanionProject?: string | { id?: string } };
      };
      const pollProjectId = readProjectId(pollData.response?.cloudaicompanionProject);
      if (pollData.done && pollProjectId) {
        return pollProjectId;
      }
    }
    throw new Error("onboardUser timed out");
  }

  if (envProjectId) {
    return envProjectId;
  }

  throw new Error(
    "Could not discover or provision a Google Cloud project. " +
      "Try setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable.",
  );
}

/**
 * Performs full OAuth login flow for Antigravity
 */
async function loginWithConfig(): Promise<void> {
  const config = ANTIGRAVITY_OAUTH_CONFIG;
  const state = crypto.randomBytes(16).toString("hex");
  const redirectUri = `http://localhost:${config.port}${config.callbackPath}`;

  const authUrl = new URL(OAUTH_AUTH_URL);
  authUrl.searchParams.set("client_id", config.clientId);
  authUrl.searchParams.set("response_type", "code");
  authUrl.searchParams.set("scope", config.scopes.join(" "));
  authUrl.searchParams.set("redirect_uri", redirectUri);
  authUrl.searchParams.set("state", state);
  authUrl.searchParams.set("access_type", "offline");
  authUrl.searchParams.set("prompt", "consent");

  console.log(`Opening browser for ${config.type} login...`);
  await openBrowser(authUrl.toString());

  const {
    code,
    state: receivedState,
    close,
  } = await startCallbackServer(config.port, config.callbackPath);
  close();

  if (receivedState !== state) {
    throw new Error("State mismatch");
  }

  const tokens = await exchangeCodeForTokens(code, redirectUri);
  const email = await getUserEmail(tokens.access_token);
  const projectId = await loadCodeAssist(tokens.access_token);

  const credential: GeminiOAuthCredential = {
    token: tokens.access_token,
    refreshToken: tokens.refresh_token,
    projectId,
    email,
    expiresAt: Date.now() + tokens.expires_in * 1000,
  };

  await saveCredential(credential, config.type);
  console.log(`Successfully logged in to ${config.type}! Credentials saved.`);
  console.log(`  Email: ${email}`);
  console.log(`  Project ID: ${projectId}`);
}

export async function completeAuthFlowWithCode(
  _provider: OAuthProviderId,
  code: string,
  explicitProjectId?: string,
): Promise<GeminiOAuthCredential> {
  const config = ANTIGRAVITY_OAUTH_CONFIG;
  const redirectUri = `http://localhost:${config.port}${config.callbackPath}`;

  const tokens = await exchangeCodeForTokens(code, redirectUri);
  const email = await getUserEmail(tokens.access_token);
  const projectId = await loadCodeAssist(tokens.access_token, explicitProjectId);

  const credential: GeminiOAuthCredential = {
    token: tokens.access_token,
    refreshToken: tokens.refresh_token,
    projectId,
    email,
    expiresAt: Date.now() + tokens.expires_in * 1000,
  };

  await saveCredential(credential, config.type);
  return credential;
}

/**
 * Login for Antigravity
 */
export async function loginAntigravity(): Promise<void> {
  return loginWithConfig();
}
