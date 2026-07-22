/**
 * OAuth login flow implementation for both Gemini CLI and Antigravity.
 *
 * - Spins up local callback server
 * - Opens browser for auth
 * - Exchanges code for tokens
 * - Calls loadCodeAssist / onboardUser to get projectId
 * - Saves credential to ~/.console/{gemini,antigravity}-creds.json
 */

import * as http from "node:http";
import * as crypto from "node:crypto";
import { exec } from "node:child_process";
import { promisify } from "node:util";
import type { GeminiOAuthCredential } from "../types/index.js";
import { saveCredential } from "./token-store.js";
import {
  OAUTH_AUTH_URL,
  OAUTH_TOKEN_URL,
  USERINFO_URL,
  GEMINI_OAUTH_CONFIG,
  ANTIGRAVITY_OAUTH_CONFIG,
  getAntigravityUserAgent,
  getGeminiCliHeaders,
} from "../constants.js";

type OAuthConfig = typeof GEMINI_OAUTH_CONFIG | typeof ANTIGRAVITY_OAUTH_CONFIG;

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
        "/Users/techclub/Documents/projects/console/.dbg/gemini-onboard-poll.env",
        "utf8",
      ),
    );
    const serverUrl =
      envText.match(/DEBUG_SERVER_URL=(.+)/)?.[1]?.trim() ?? "http://127.0.0.1:7777/event";
    const sessionId = envText.match(/DEBUG_SESSION_ID=(.+)/)?.[1]?.trim() ?? "gemini-onboard-poll";
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
    const server = http.createServer((req, res) => {
      if (req.url?.startsWith(callbackPath)) {
        const url = new URL(req.url, `http://localhost:${port}`);
        const code = url.searchParams.get("code");
        const state = url.searchParams.get("state");

        res.writeHead(200, { "Content-Type": "text/html" });
        res.end(`
          <!DOCTYPE html>
          <html>
          <head><title>Login Successful</title></head>
          <body>
            <h1>Successfully authenticated!</h1>
            <p>You can close this tab now.</p>
          </body>
          </html>
        `);

        if (code && state) {
          resolve({ code, state, close: () => server.close() });
        } else {
          reject(new Error("Invalid callback: missing code or state"));
        }
      }
    });

    server.listen(port, () => {
      console.log(`Callback server listening on http://localhost:${port}`);
    });

    server.on("error", reject);
  });
}

/**
 * Exchanges authorization code for tokens
 */
async function exchangeCodeForTokens(
  config: OAuthConfig,
  code: string,
  redirectUri: string,
): Promise<{ access_token: string; refresh_token: string; expires_in: number }> {
  const body = new URLSearchParams({
    client_id: config.clientId,
    client_secret: config.clientSecret,
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
  ideType: "GEMINI_CLI" | "ANTIGRAVITY",
): Promise<string> {
  const envProjectId = process.env.GOOGLE_CLOUD_PROJECT || process.env.GOOGLE_CLOUD_PROJECT_ID;
  const baseEndpoint =
    ideType === "ANTIGRAVITY"
      ? "https://daily-cloudcode-pa.googleapis.com"
      : "https://cloudcode-pa.googleapis.com";
  const endpoint = `${baseEndpoint}/v1internal:loadCodeAssist`;

  const headers: Record<string, string> = {
    Authorization: `Bearer ${accessToken}`,
    "Content-Type": "application/json",
    ...(ideType === "ANTIGRAVITY"
      ? { "User-Agent": getAntigravityUserAgent() }
      : getGeminiCliHeaders()),
  };

  const response = await fetch(endpoint, {
    method: "POST",
    headers,
    body: JSON.stringify({
      ...(ideType === "GEMINI_CLI" && envProjectId
        ? { cloudaicompanionProject: envProjectId }
        : {}),
      metadata: {
        ideType: ideType === "ANTIGRAVITY" ? "ANTIGRAVITY" : "IDE_UNSPECIFIED",
        platform: "PLATFORM_UNSPECIFIED",
        pluginType: "GEMINI",
        ...(ideType === "GEMINI_CLI" && envProjectId ? { duetProject: envProjectId } : {}),
      },
    }),
  });

  // #region debug-point B:load-code-assist-response
  await reportDebugEvent(
    "B",
    "login.ts:loadCodeAssist",
    "[DEBUG] loadCodeAssist response received",
    {
      ideType,
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
    let errorPayload: unknown;
    try {
      errorPayload = await response.clone().json();
    } catch {
      errorPayload = undefined;
    }

    if (ideType === "GEMINI_CLI" && isVpcScAffectedUser(errorPayload)) {
      data = { currentTier: { id: TIER_STANDARD } };
    } else {
      const detail = await response.text().catch(() => "");
      throw new Error(`loadCodeAssist failed (${response.status}): ${detail}`);
    }
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
    ideType,
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
        "This account requires setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable. " +
          "See https://goo.gle/gemini-cli-auth-docs#workspace-gca",
      );
    }
    // For free tier or legacy tier, proceed to provision
  }

  // Additional check for non-free tiers
  if (
    ideType === "GEMINI_CLI" &&
    tierId !== TIER_FREE &&
    tierId !== "legacy-tier" &&
    !envProjectId
  ) {
    throw new Error(
      "This account requires setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable. " +
        "See https://goo.gle/gemini-cli-auth-docs#workspace-gca",
    );
  }

  return onboardUser(accessToken, ideType, tierId, envProjectId);
}

/**
 * Calls onboardUser API to provision a project (polling until done)
 */
async function onboardUser(
  accessToken: string,
  ideType: "GEMINI_CLI" | "ANTIGRAVITY",
  tierId?: string,
  envProjectId?: string,
): Promise<string> {
  const baseEndpoint =
    ideType === "ANTIGRAVITY"
      ? "https://daily-cloudcode-pa.googleapis.com"
      : "https://cloudcode-pa.googleapis.com";
  const endpoint = `${baseEndpoint}/v1internal:onboardUser`;

  const headers: Record<string, string> = {
    Authorization: `Bearer ${accessToken}`,
    "Content-Type": "application/json",
    ...(ideType === "ANTIGRAVITY"
      ? { "User-Agent": getAntigravityUserAgent() }
      : getGeminiCliHeaders()),
  };

  const body: Record<string, unknown> = {
    metadata: {
      ideType: ideType === "ANTIGRAVITY" ? "ANTIGRAVITY" : "IDE_UNSPECIFIED",
      platform: "PLATFORM_UNSPECIFIED",
      pluginType: "GEMINI",
    },
  };

  if (tierId) {
    body.tierId = tierId;
  }

  if (
    ideType === "GEMINI_CLI" &&
    tierId !== TIER_FREE &&
    tierId !== "legacy-tier" &&
    envProjectId
  ) {
    body.cloudaicompanionProject = envProjectId;
    (body.metadata as Record<string, unknown>).duetProject = envProjectId;
  }

  const response = await fetch(endpoint, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });

  // #region debug-point A:onboard-user-response
  await reportDebugEvent("A", "login.ts:onboardUser", "[DEBUG] onboardUser response received", {
    ideType,
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
    ideType,
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
      const pollEndpoint = `${baseEndpoint}/v1internal/${operation.name}`;
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
      "Try setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable. " +
      "See https://goo.gle/gemini-cli-auth-docs#workspace-gca",
  );
}

/**
 * Performs full OAuth login flow for a given config
 */
async function loginWithConfig(config: OAuthConfig): Promise<void> {
  const state = crypto.randomBytes(16).toString("hex");
  const redirectUri = `http://127.0.0.1:${config.port}${config.callbackPath}`;

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

  const tokens = await exchangeCodeForTokens(config, code, redirectUri);
  const email = await getUserEmail(tokens.access_token);
  const projectId = await loadCodeAssist(tokens.access_token, config.ideType);

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
  provider: "gemini" | "antigravity",
  code: string,
): Promise<GeminiOAuthCredential> {
  const config = provider === "gemini" ? GEMINI_OAUTH_CONFIG : ANTIGRAVITY_OAUTH_CONFIG;
  const redirectUri = `http://127.0.0.1:${config.port}${config.callbackPath}`;

  const tokens = await exchangeCodeForTokens(config, code, redirectUri);
  const email = await getUserEmail(tokens.access_token);
  const projectId = await loadCodeAssist(tokens.access_token, config.ideType);

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
 * Login for Gemini CLI
 */
export async function loginGemini(): Promise<void> {
  return loginWithConfig(GEMINI_OAUTH_CONFIG);
}

/**
 * Login for Antigravity
 */
export async function loginAntigravity(): Promise<void> {
  return loginWithConfig(ANTIGRAVITY_OAUTH_CONFIG);
}
