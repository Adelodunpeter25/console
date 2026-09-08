/**
 * Devin OAuth: PKCE authorization URL, CLI token exchange, credential storage.
 *
 * Mirrors oh-my-pi's `registry/oauth/devin.ts`. The CLI token returned by
 * `api.devin.ai/auth/cli/token` is the bearer used for the streaming API at
 * `server.codeium.com` (see stream-fn.ts).
 */
import * as crypto from "node:crypto";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import {
  DEVIN_API_URL,
  DEVIN_CALLBACK_PATH,
  DEVIN_CALLBACK_PORT,
  DEVIN_FALLBACK_EXPIRES_MS,
  DEVIN_OAUTH_AUTHORIZE_URL,
  DEVIN_TOKEN_URL,
} from "./constants.js";

export interface DevinOAuthCredential {
  /** Session token returned by the CLI token exchange. */
  access_token: string;
  /** Same as access_token — oh-my-pi uses one string for both. */
  refresh_token: string;
  /** Epoch ms when the token expires (best-effort, JWT-derived). */
  expiresAt: number;
  email?: string;
}

export interface ParsedDevinCredential {
  accessToken: string;
  refreshToken: string;
  expiresAtMs: number;
  email?: string;
}

function credentialPath(): string {
  return process.env.DEVIN_CREDENTIALS_PATH ?? path.join(os.homedir(), ".console", "devin-creds.json");
}

export function decodeJwtPayload(token: string): Record<string, unknown> | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    return JSON.parse(Buffer.from(parts[1]!, "base64url").toString("utf8")) as Record<string, unknown>;
  } catch {
    return null;
  }
}

/**
 * Read the `exp` claim from a JWT and convert to ms. Falls back to a
 * long-lived estimate if the token isn't a JWT.
 */
export function getTokenExpiry(token: string): number {
  const payload = decodeJwtPayload(token);
  if (payload && typeof payload.exp === "number" && Number.isFinite(payload.exp)) {
    return payload.exp * 1000 - 5 * 60 * 1000; // 5-minute skew
  }
  return Date.now() + DEVIN_FALLBACK_EXPIRES_MS;
}

export function generateDevinPkce(): { verifier: string; challenge: string } {
  const verifier = crypto.randomBytes(32).toString("base64url");
  const challenge = new Bun.CryptoHasher("sha256").update(verifier).digest("base64url");
  return { verifier, challenge };
}

export function createDevinAuthorizationUrl(args: {
  state: string;
  verifierChallenge: string;
}): { authUrl: string; redirectUri: string } {
  const redirectUri = `http://127.0.0.1:${DEVIN_CALLBACK_PORT}${DEVIN_CALLBACK_PATH}`;
  const params = new URLSearchParams({
    redirect_uri: redirectUri,
    state: args.state,
    prompt: "select_account",
    code_challenge: args.verifierChallenge,
    code_challenge_method: "S256",
  });
  return {
    authUrl: `${DEVIN_OAUTH_AUTHORIZE_URL}?${params.toString()}`,
    redirectUri,
  };
}

function tokenError(status: number, body: string): Error {
  return new Error(`Devin OAuth token exchange failed (${status}): ${body.trim() || "unknown error"}`);
}

export async function exchangeDevinCode(
  code: string,
  verifier: string,
  redirectUri: string,
): Promise<DevinOAuthCredential> {
  const response = await fetch(DEVIN_TOKEN_URL, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ code, code_verifier: verifier, redirect_uri: redirectUri }),
    signal: AbortSignal.timeout(15_000),
  });
  if (!response.ok) throw tokenError(response.status, await response.text().catch(() => ""));
  const data = (await response.json()) as { token?: unknown };
  if (typeof data.token !== "string" || data.token.length === 0) {
    throw new Error("Devin token exchange returned an empty token.");
  }
  const expiresAt = getTokenExpiry(data.token);
  return {
    access_token: data.token,
    refresh_token: data.token,
    expiresAt,
  };
}

export function parseDevinCredential(raw: DevinOAuthCredential): ParsedDevinCredential {
  if (!raw.access_token) {
    throw new Error("Invalid Devin credential. Please login again.");
  }
  return {
    accessToken: raw.access_token,
    refreshToken: raw.refresh_token ?? raw.access_token,
    expiresAtMs: raw.expiresAt,
    email: raw.email,
  };
}

export async function loadDevinCredential(): Promise<ParsedDevinCredential> {
  const envToken = process.env.DEVIN_OAUTH_TOKEN;
  if (envToken) {
    return parseDevinCredential({
      access_token: envToken,
      refresh_token: envToken,
      expiresAt: getTokenExpiry(envToken),
    });
  }
  const raw = JSON.parse(await fs.readFile(credentialPath(), "utf8")) as DevinOAuthCredential;
  return parseDevinCredential(raw);
}

export async function saveDevinCredential(credential: DevinOAuthCredential): Promise<void> {
  const filePath = credentialPath();
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(credential, null, 2), "utf8");
}

export async function devinCredentialExists(): Promise<boolean> {
  if (process.env.DEVIN_OAUTH_TOKEN) return true;
  try {
    await fs.access(credentialPath());
    return true;
  } catch {
    return false;
  }
}

/**
 * Devin's CLI token doesn't have a refresh path — when it expires, the user
 * has to log in again. This function exists to satisfy the `refreshIfNeeded`
 * contract used by other providers; it just returns the credential unchanged.
 */
export async function refreshDevinIfNeeded(
  credential: ParsedDevinCredential,
): Promise<ParsedDevinCredential> {
  if (Date.now() + 5 * 60_000 < credential.expiresAtMs) return credential;
  // Within the 5-minute expiry window — try to load from disk; if it has
  // already been rotated externally (manual re-login), pick that up.
  try {
    return await loadDevinCredential();
  } catch {
    return credential;
  }
}

/** Used by auth.service.ts — Devin's API base for the OAuth-callback return shape. */
export const DEVIN_API_BASE = DEVIN_API_URL;
