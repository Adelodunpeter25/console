/**
 * Refreshes an OAuth token if it is close to expiry.
 *
 * Uses Google's standard refresh_token grant.
 * In-flight deduplication ensures concurrent callers share a single refresh request.
 */
import type { GeminiOAuthCredential, ParsedCredential } from "../types/index.js";
import { parseCredential, saveCredential, type CredentialType } from "./token-store.js";
import {
  OAUTH_TOKEN_URL,
  REFRESH_SKEW_MS,
  GEMINI_CLI_CLIENT_ID,
  GEMINI_CLI_CLIENT_SECRET,
  ANTIGRAVITY_CLIENT_ID,
  ANTIGRAVITY_CLIENT_SECRET,
} from "../constants.js";

interface TokenResponse {
  access_token: string;
  expires_in: number;
  token_type?: string;
}

const inflight = new Map<CredentialType, Promise<ParsedCredential> | null>();

async function doRefresh(
  cred: ParsedCredential,
  type: CredentialType,
  signal?: AbortSignal,
): Promise<ParsedCredential> {
  if (!cred.refreshToken) {
    throw new Error("OAuth token expired and no refresh_token available. Please login again.");
  }

  const clientId = type === "antigravity" ? ANTIGRAVITY_CLIENT_ID : GEMINI_CLI_CLIENT_ID;
  const clientSecret =
    type === "antigravity" ? ANTIGRAVITY_CLIENT_SECRET : GEMINI_CLI_CLIENT_SECRET;

  const body = new URLSearchParams({
    client_id: clientId,
    client_secret: clientSecret,
    refresh_token: cred.refreshToken,
    grant_type: "refresh_token",
  });

  const response = await fetch(OAUTH_TOKEN_URL, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
    signal,
  });

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(
      `OAuth token refresh failed (${response.status}): ${detail}\nPlease login again.`,
    );
  }

  const data = (await response.json()) as TokenResponse;
  const expiresAtMs = Date.now() + data.expires_in * 1000;

  const updated: GeminiOAuthCredential = {
    token: data.access_token,
    refreshToken: cred.refreshToken,
    projectId: cred.projectId,
    email: cred.email,
    expiresAt: expiresAtMs,
  };

  await saveCredential(updated, type);
  return parseCredential(updated);
}

/**
 * Returns the credential, refreshing it first if it's within REFRESH_SKEW_MS of expiry.
 * Concurrent callers share a single in-flight refresh promise per credential type.
 */
export async function refreshIfNeeded(
  cred: ParsedCredential,
  type: CredentialType = "gemini",
  signal?: AbortSignal,
): Promise<ParsedCredential> {
  const needsRefresh =
    cred.expiresAtMs !== undefined && Date.now() + REFRESH_SKEW_MS >= cred.expiresAtMs;

  if (!needsRefresh) return cred;

  // Dedup concurrent refreshes per type
  let currentInflight = inflight.get(type);
  if (currentInflight) return currentInflight;

  const promise = doRefresh(cred, type, signal).finally(() => {
    inflight.set(type, null);
  });

  inflight.set(type, promise);
  return promise;
}
