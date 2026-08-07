/**
 * Codebuff device-code login flow — replicates the official Codebuff/Freebuff
 * CLI authentication protocol so console users can log in with their Codebuff
 * account and use the free tier:
 *
 *   1. POST  {base}/api/auth/cli/code  with a fingerprintId
 *           → { loginUrl, fingerprintHash, expiresAt }
 *   2. User opens loginUrl in a browser and signs in on codebuff.com
 *   3. GET   {base}/api/auth/cli/status?fingerprintId&fingerprintHash&expiresAt
 *           → { user: { id, name, email, authToken, ... } }
 *   4. The authToken is stored and used as `Authorization: Bearer` on every
 *      chat-completions request.
 */
import { randomBytes } from "node:crypto";

import { CODEBUFF_BASE_URL } from "./constants.js";
import { saveCodebuffCredential, type CodebuffCredential } from "./creds.js";

export interface CodebuffLoginCode {
  loginUrl: string;
  fingerprintId: string;
  fingerprintHash: string;
  /** Epoch-ms number per the real API, but accept string for robustness. */
  expiresAt: number | string;
}

export interface CodebuffLoginStatus {
  loggedIn: boolean;
  credential?: CodebuffCredential;
}

/** Fingerprint id. The CLI derives a hardware hash; any unique string works. */
export function generateFingerprintId(): string {
  return `console-${randomBytes(8).toString("base64url")}`;
}

/**
 * Start a login: request a login code from the Codebuff backend.
 * Returns the URL the user must open, plus the polling params.
 */
export async function startCodebuffLogin(
  fingerprintId = generateFingerprintId(),
): Promise<CodebuffLoginCode> {
  const response = await fetch(`${CODEBUFF_BASE_URL}/api/auth/cli/code`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ fingerprintId }),
  });

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(
      `Codebuff login code request failed (${response.status}): ${detail}`,
    );
  }

  const data = (await response.json()) as Partial<CodebuffLoginCode>;
  if (!data.loginUrl || !data.fingerprintHash || !data.expiresAt) {
    throw new Error(
      "Codebuff login response missing loginUrl/fingerprintHash/expiresAt.",
    );
  }

  return {
    loginUrl: data.loginUrl,
    fingerprintId,
    fingerprintHash: data.fingerprintHash,
    expiresAt: data.expiresAt,
  };
}

/**
 * Poll login status. Returns loggedIn:false until the user completes the
 * browser login; on success stores the credential and returns it.
 *
 * The backend returns 401 ("Authentication failed") while the user has not
 * yet approved the login — that is the normal "keep polling" state, exactly
 * like the official CLI treats it, so it maps to loggedIn:false rather than
 * an error. Other HTTP errors are real failures and throw.
 */
export async function pollCodebuffLogin(params: {
  fingerprintId: string;
  fingerprintHash: string;
  expiresAt: number | string;
}): Promise<CodebuffLoginStatus> {
  const query = new URLSearchParams({
    fingerprintId: params.fingerprintId,
    fingerprintHash: params.fingerprintHash,
    expiresAt: String(params.expiresAt),
  });
  const response = await fetch(
    `${CODEBUFF_BASE_URL}/api/auth/cli/status?${query.toString()}`,
    { headers: { Accept: "application/json" } },
  );

  // 401 = not approved yet → keep polling.
  if (response.status === 401) {
    return { loggedIn: false };
  }

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(
      `Codebuff login status request failed (${response.status}): ${detail}`,
    );
  }

  const data = (await response.json()) as {
    user?: Record<string, unknown>;
  };

  const user = data.user;
  if (!user || typeof user !== "object") {
    return { loggedIn: false };
  }

  const authToken =
    typeof user.authToken === "string" ? user.authToken : undefined;
  if (!authToken) {
    return { loggedIn: false };
  }

  const credential: CodebuffCredential = {
    authToken,
    ...(typeof user.id === "string" ? { id: user.id } : {}),
    ...(typeof user.name === "string" ? { name: user.name } : {}),
    ...(typeof user.email === "string" ? { email: user.email } : {}),
  };

  await saveCodebuffCredential(credential);
  return { loggedIn: true, credential };
}
