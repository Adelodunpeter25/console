/**
 * Reads and writes OAuth credential files for both Gemini and Antigravity.
 *
 * Default directory: ~/.console
 *
 * Gemini credentials: ~/.console/gemini-creds.json
 * Antigravity credentials: ~/.console/antigravity-creds.json
 *
 * Also falls back to Gemini CLI's default ~/.gemini/oauth_creds.json for compatibility.
 */
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import type { GeminiOAuthCredential, ParsedCredential } from "../types/index.js";

export type CredentialType = "gemini" | "antigravity";

function credentialCandidates(type: CredentialType): string[] {
  const custom = process.env[`${type.toUpperCase()}_CREDENTIALS_PATH`];
  const defaultPath = path.join(os.homedir(), ".console", `${type}-creds.json`);

  // For Gemini, also check the default Gemini CLI path for compatibility
  if (type === "gemini") {
    const geminiCliPath1 = path.join(os.homedir(), ".gemini", "oauth_creds.json");
    const geminiCliPath2 = path.join(os.homedir(), ".config", "gemini", "oauth_creds.json");
    return custom ? [custom, defaultPath, geminiCliPath1, geminiCliPath2] : [defaultPath, geminiCliPath1, geminiCliPath2];
  }

  // For Antigravity, only use our default path (no legacy)
  return custom ? [custom, defaultPath] : [defaultPath];
}

function normalizeExpiryMs(value: unknown): number | undefined {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) return undefined;
  // If < 10 billion it's epoch seconds, otherwise epoch ms
  return value < 10_000_000_000 ? value * 1000 : value;
}

export function parseCredential(raw: GeminiOAuthCredential): ParsedCredential {
  const accessToken = raw.token || raw.access_token;
  const projectId = raw.projectId ?? raw.project_id;

  if (!accessToken) {
    throw new Error(
      "Missing access token in OAuth credential. Please login again.",
    );
  }
  if (!projectId) {
    throw new Error(
      "Missing projectId in OAuth credential. Please login again.",
    );
  }

  return {
    accessToken,
    projectId,
    refreshToken: raw.refreshToken ?? raw.refresh ?? raw.refresh_token,
    expiresAtMs: normalizeExpiryMs(raw.expiresAt ?? raw.expires ?? raw.expiry_date),
    email: raw.email ?? undefined,
  };
}

const credentialPaths = new Map<CredentialType, string | null>();

export async function loadCredential(type: CredentialType = "gemini"): Promise<ParsedCredential> {
  const candidates = credentialCandidates(type);

  for (const filePath of candidates) {
    let content: string;
    try {
      content = await fs.readFile(filePath, "utf-8");
    } catch {
      continue;
    }

    let raw: GeminiOAuthCredential;
    try {
      raw = JSON.parse(content) as GeminiOAuthCredential;
    } catch {
      throw new Error(`Invalid JSON in OAuth credential file: ${filePath}`);
    }

    credentialPaths.set(type, filePath);
    return parseCredential(raw);
  }

  throw new Error(
    `No OAuth credential file found for ${type}. Tried:\n${candidates.map((p) => `  ${p}`).join("\n")}`,
  );
}

export async function saveCredential(raw: GeminiOAuthCredential, type: CredentialType = "gemini"): Promise<void> {
  let filePath = credentialPaths.get(type);
  if (!filePath) {
    filePath = credentialCandidates(type)[0]!;
    credentialPaths.set(type, filePath);
  }

  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(raw, null, 2), "utf-8");
}

export async function credentialExists(type: CredentialType = "gemini"): Promise<boolean> {
  const candidates = credentialCandidates(type);
  for (const filePath of candidates) {
    try {
      await fs.access(filePath);
      return true;
    } catch {
      continue;
    }
  }
  return false;
}
