import * as crypto from "node:crypto";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import {
  CODEX_ACCOUNT_CLAIM,
  CODEX_AUTHORIZE_URL,
  CODEX_CLIENT_ID,
  CODEX_CALLBACK_PATH,
  CODEX_CALLBACK_PORT,
  CODEX_PROFILE_CLAIM,
  CODEX_SCOPE,
  CODEX_TOKEN_URL,
  CODEX_REFRESH_SKEW_MS,
} from "./constants.js";

export interface CodexOAuthCredential {
  access_token: string;
  refresh_token: string;
  expiresAt: number;
  accountId: string;
  email?: string;
  planType?: string;
}

export interface ParsedCodexCredential {
  accessToken: string;
  refreshToken: string;
  expiresAtMs: number;
  accountId: string;
  email?: string;
}

interface TokenResponse {
  access_token?: string;
  refresh_token?: string;
  id_token?: string;
  expires_in?: number;
}

function credentialPath(): string {
  return process.env.CODEX_CREDENTIALS_PATH ?? path.join(os.homedir(), ".console", "codex-creds.json");
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

export function getCodexTokenProfile(
  accessToken: string,
  idToken?: string,
): { accountId?: string; email?: string; planType?: string } {
  const payload = decodeJwtPayload(accessToken);
  const idPayload = idToken ? decodeJwtPayload(idToken) : null;
  const auth = payload?.[CODEX_ACCOUNT_CLAIM] as Record<string, unknown> | undefined;
  const idAuth = idPayload?.[CODEX_ACCOUNT_CLAIM] as Record<string, unknown> | undefined;
  const profile = payload?.[CODEX_PROFILE_CLAIM] as Record<string, unknown> | undefined;
  return {
    accountId: typeof auth?.chatgpt_account_id === "string" ? auth.chatgpt_account_id : undefined,
    email: typeof profile?.email === "string" ? profile.email.trim().toLowerCase() : undefined,
    planType:
      typeof (auth?.chatgpt_plan_type ?? idAuth?.chatgpt_plan_type) === "string"
        ? String(auth?.chatgpt_plan_type ?? idAuth?.chatgpt_plan_type).trim().toLowerCase()
        : undefined,
  };
}

export function generateCodexPkce(): { verifier: string; challenge: string } {
  const verifier = crypto.randomBytes(32).toString("base64url");
  const challenge = new Bun.CryptoHasher("sha256").update(verifier).digest("base64url");
  return { verifier, challenge };
}

export function createCodexAuthorizationUrl(args: {
  state: string;
  verifierChallenge: string;
}): { authUrl: string; redirectUri: string } {
  const redirectUri = `http://localhost:${CODEX_CALLBACK_PORT}${CODEX_CALLBACK_PATH}`;
  const url = new URL(CODEX_AUTHORIZE_URL);
  url.search = new URLSearchParams({
    response_type: "code",
    client_id: CODEX_CLIENT_ID,
    redirect_uri: redirectUri,
    scope: CODEX_SCOPE,
    code_challenge: args.verifierChallenge,
    code_challenge_method: "S256",
    state: args.state,
    id_token_add_organizations: "true",
    codex_cli_simplified_flow: "true",
    originator: "pi",
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
  return new Error(`Codex OAuth request failed (${status}): ${detail || "unknown error"}`);
}

async function postToken(body: URLSearchParams): Promise<TokenResponse> {
  const response = await fetch(CODEX_TOKEN_URL, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
    signal: AbortSignal.timeout(15_000),
  });
  if (!response.ok) throw tokenError(response.status, await response.text().catch(() => ""));
  return (await response.json()) as TokenResponse;
}

export async function exchangeCodexCode(code: string, verifier: string, redirectUri: string): Promise<CodexOAuthCredential> {
  const data = await postToken(
    new URLSearchParams({
      grant_type: "authorization_code",
      client_id: CODEX_CLIENT_ID,
      code,
      code_verifier: verifier,
      redirect_uri: redirectUri,
    }),
  );
  if (!data.access_token || !data.refresh_token || typeof data.expires_in !== "number") {
    throw new Error("Codex OAuth response is missing required token fields.");
  }
  const profile = getCodexTokenProfile(data.access_token, data.id_token);
  if (!profile.accountId) throw new Error("Codex OAuth response did not include a ChatGPT account ID.");
  return {
    access_token: data.access_token,
    refresh_token: data.refresh_token,
    expiresAt: Date.now() + data.expires_in * 1000,
    accountId: profile.accountId,
    email: profile.email,
    planType: profile.planType,
  };
}

async function refreshCodexToken(credential: ParsedCodexCredential): Promise<ParsedCodexCredential> {
  const data = await postToken(
    new URLSearchParams({
      grant_type: "refresh_token",
      client_id: CODEX_CLIENT_ID,
      refresh_token: credential.refreshToken,
    }),
  );
  if (!data.access_token || !data.refresh_token || typeof data.expires_in !== "number") {
    throw new Error("Codex token refresh response is missing required fields.");
  }
  const profile = getCodexTokenProfile(data.access_token);
  const updated: CodexOAuthCredential = {
    access_token: data.access_token,
    refresh_token: data.refresh_token,
    expiresAt: Date.now() + data.expires_in * 1000,
    accountId: profile.accountId ?? credential.accountId,
    email: profile.email ?? credential.email,
  };
  await saveCodexCredential(updated);
  return parseCodexCredential(updated);
}

export function parseCodexCredential(raw: CodexOAuthCredential): ParsedCodexCredential {
  if (!raw.access_token || !raw.refresh_token || !raw.accountId) {
    throw new Error("Invalid Codex credential. Please login again.");
  }
  return {
    accessToken: raw.access_token,
    refreshToken: raw.refresh_token,
    expiresAtMs: raw.expiresAt,
    accountId: raw.accountId,
    email: raw.email,
  };
}

export async function loadCodexCredential(): Promise<ParsedCodexCredential> {
  const envToken = process.env.OPENAI_CODEX_OAUTH_TOKEN;
  if (envToken) {
    const profile = getCodexTokenProfile(envToken);
    if (!profile.accountId) throw new Error("OPENAI_CODEX_OAUTH_TOKEN has no ChatGPT account ID.");
    return {
      accessToken: envToken,
      refreshToken: "",
      expiresAtMs: Number.MAX_SAFE_INTEGER,
      accountId: profile.accountId,
      email: profile.email,
    };
  }
  const raw = JSON.parse(await fs.readFile(credentialPath(), "utf8")) as CodexOAuthCredential;
  return parseCodexCredential(raw);
}

export async function saveCodexCredential(credential: CodexOAuthCredential): Promise<void> {
  const filePath = credentialPath();
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(credential, null, 2), "utf8");
}

export async function codexCredentialExists(): Promise<boolean> {
  if (process.env.OPENAI_CODEX_OAUTH_TOKEN) return true;
  try {
    await fs.access(credentialPath());
    return true;
  } catch {
    return false;
  }
}

export async function refreshCodexIfNeeded(credential: ParsedCodexCredential): Promise<ParsedCodexCredential> {
  if (!credential.refreshToken || Date.now() + CODEX_REFRESH_SKEW_MS < credential.expiresAtMs) return credential;
  return refreshCodexToken(credential);
}
