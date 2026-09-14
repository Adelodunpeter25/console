/**
 * Claude OAuth: PKCE authorization URL, JSON token exchange, credential
 * storage. Mirrors pi's `auth/oauth/anthropic.ts` and console's
 * `codex/oauth.ts` structure.
 */
import * as crypto from "node:crypto";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import {
  CLAUDE_AUTHORIZE_URL,
  CLAUDE_CLIENT_ID,
  CLAUDE_REFRESH_SKEW_MS,
  CLAUDE_SCOPE,
  CLAUDE_TOKEN_URL,
  claudeRedirectUri,
} from "./constants.js";

export interface ClaudeOAuthCredential {
  access_token: string;
  refresh_token: string;
  expiresAt: number;
  email?: string;
}

export interface ParsedClaudeCredential {
  accessToken: string;
  refreshToken: string;
  expiresAtMs: number;
  email?: string;
}

interface TokenResponse {
  access_token?: string;
  refresh_token?: string;
  expires_in?: number;
}

function credentialPath(): string {
  return process.env.CLAUDE_CREDENTIALS_PATH ?? path.join(os.homedir(), ".console", "claude-creds.json");
}

export function generateClaudePkce(): { verifier: string; challenge: string } {
  const verifier = crypto.randomBytes(32).toString("base64url");
  const challenge = new Bun.CryptoHasher("sha256").update(verifier).digest("base64url");
  return { verifier, challenge };
}

export function createClaudeAuthorizationUrl(args: {
  state: string;
  verifierChallenge: string;
}): { authUrl: string; redirectUri: string } {
  const redirectUri = claudeRedirectUri();
  const url = new URL(CLAUDE_AUTHORIZE_URL);
  url.search = new URLSearchParams({
    code: "true",
    client_id: CLAUDE_CLIENT_ID,
    response_type: "code",
    redirect_uri: redirectUri,
    scope: CLAUDE_SCOPE,
    code_challenge: args.verifierChallenge,
    code_challenge_method: "S256",
    state: args.state,
  }).toString();
  return { authUrl: url.toString(), redirectUri };
}

function tokenError(status: number, body: string): Error {
  let detail = body.trim();
  try {
    const parsed = JSON.parse(detail) as { error?: unknown; error_description?: unknown; message?: unknown };
    detail = String(parsed.error_description ?? parsed.error ?? parsed.message ?? detail);
  } catch {
    // Keep the raw response when it is not JSON.
  }
  return new Error(`Claude OAuth request failed (${status}): ${detail || "unknown error"}`);
}

async function postToken(body: Record<string, string>): Promise<TokenResponse> {
  const response = await fetch(CLAUDE_TOKEN_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(15_000),
  });
  if (!response.ok) throw tokenError(response.status, await response.text().catch(() => ""));
  return (await response.json()) as TokenResponse;
}

/**
 * Best-effort account identity lookup via the Claude Code bootstrap
 * endpoint (mirrors oh-my-pi's `anthropic-identity` hook). Returns the
 * account email when available, undefined otherwise — never throws.
 */
async function fetchBootstrapEmail(accessToken: string): Promise<string | undefined> {
  try {
    const response = await fetch(
      "https://api.anthropic.com/api/claude_cli/bootstrap?entrypoint=cli&model=claude-sonnet-4-6",
      {
        method: "GET",
        headers: {
          Accept: "application/json, text/plain, */*",
          Authorization: `Bearer ${accessToken}`,
          "Content-Type": "application/json",
          "anthropic-beta": "oauth-2025-04-20",
        },
        signal: AbortSignal.timeout(15_000),
      },
    );
    if (!response.ok) return undefined;
    const data = (await response.json()) as {
      oauth_account?: { account_email?: unknown };
    };
    const email = data.oauth_account?.account_email;
    return typeof email === "string" && email.length > 0 ? email : undefined;
  } catch {
    return undefined;
  }
}

export async function exchangeClaudeCode(
  code: string,
  state: string,
  verifier: string,
  redirectUri: string,
): Promise<ClaudeOAuthCredential> {
  const data = await postToken({
    grant_type: "authorization_code",
    client_id: CLAUDE_CLIENT_ID,
    code,
    state,
    redirect_uri: redirectUri,
    code_verifier: verifier,
  });
  if (!data.access_token || !data.refresh_token || typeof data.expires_in !== "number") {
    throw new Error("Claude OAuth response is missing required token fields.");
  }
  return {
    access_token: data.access_token,
    refresh_token: data.refresh_token,
    expiresAt: Date.now() + data.expires_in * 1000,
    email: await fetchBootstrapEmail(data.access_token),
  };
}

async function refreshClaudeToken(credential: ParsedClaudeCredential): Promise<ParsedClaudeCredential> {
  const data = await postToken({
    grant_type: "refresh_token",
    client_id: CLAUDE_CLIENT_ID,
    refresh_token: credential.refreshToken,
  });
  if (!data.access_token || !data.refresh_token || typeof data.expires_in !== "number") {
    throw new Error("Claude token refresh response is missing required fields.");
  }
  const updated: ClaudeOAuthCredential = {
    access_token: data.access_token,
    refresh_token: data.refresh_token,
    expiresAt: Date.now() + data.expires_in * 1000,
    email: credential.email ?? (await fetchBootstrapEmail(data.access_token)),
  };
  await saveClaudeCredential(updated);
  return parseClaudeCredential(updated);
}

export function parseClaudeCredential(raw: ClaudeOAuthCredential): ParsedClaudeCredential {
  if (!raw.access_token || !raw.refresh_token) {
    throw new Error("Invalid Claude credential. Please login again.");
  }
  return {
    accessToken: raw.access_token,
    refreshToken: raw.refresh_token,
    expiresAtMs: raw.expiresAt,
    email: raw.email,
  };
}

export async function loadClaudeCredential(): Promise<ParsedClaudeCredential> {
  const envToken = process.env.CLAUDE_OAUTH_TOKEN ?? process.env.ANTHROPIC_OAUTH_TOKEN;
  if (envToken) {
    return {
      accessToken: envToken,
      refreshToken: "",
      expiresAtMs: Number.MAX_SAFE_INTEGER,
      email: undefined,
    };
  }
  const raw = JSON.parse(await fs.readFile(credentialPath(), "utf8")) as ClaudeOAuthCredential;
  return parseClaudeCredential(raw);
}

export async function saveClaudeCredential(credential: ClaudeOAuthCredential): Promise<void> {
  const filePath = credentialPath();
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(credential, null, 2), "utf8");
}

export async function claudeCredentialExists(): Promise<boolean> {
  if (process.env.CLAUDE_OAUTH_TOKEN ?? process.env.ANTHROPIC_OAUTH_TOKEN) return true;
  try {
    await fs.access(credentialPath());
    return true;
  } catch {
    return false;
  }
}

export async function refreshClaudeIfNeeded(credential: ParsedClaudeCredential): Promise<ParsedClaudeCredential> {
  if (!credential.refreshToken || Date.now() + CLAUDE_REFRESH_SKEW_MS < credential.expiresAtMs) return credential;
  return refreshClaudeToken(credential);
}
