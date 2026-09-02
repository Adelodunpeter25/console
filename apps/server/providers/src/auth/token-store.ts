/**
 * Reads and writes OAuth credential files for Antigravity.
 *
 * Default directory: ~/.console
 * Antigravity credentials: ~/.console/antigravity-creds.json
 */
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import type { GeminiOAuthCredential, ParsedCredential } from "@/providers/src/types/index.js";
import type { OAuthProviderId } from "@console/types";

export type CredentialType = OAuthProviderId;

function credentialCandidates(type: CredentialType): string[] {
  const custom = process.env[`${type.toUpperCase()}_CREDENTIALS_PATH`];
  const defaultPath = path.join(os.homedir(), ".console", `${type}-creds.json`);
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
    throw new Error("Missing access token in OAuth credential. Please login again.");
  }
  if (!projectId) {
    throw new Error("Missing projectId in OAuth credential. Please login again.");
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

export async function loadCredential(type: CredentialType = "antigravity"): Promise<ParsedCredential> {
  const cachedPath = credentialPaths.get(type);
  const rawCandidates = credentialCandidates(type);
  const candidates = cachedPath ? [cachedPath, ...rawCandidates.filter((p) => p !== cachedPath)] : rawCandidates;

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

export async function saveCredential(
  raw: GeminiOAuthCredential,
  type: CredentialType = "antigravity",
): Promise<void> {
  let filePath = credentialPaths.get(type);
  if (!filePath) {
    filePath = credentialCandidates(type)[0]!;
    credentialPaths.set(type, filePath);
  }

  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(raw, null, 2), "utf-8");
}

export async function credentialExists(type: CredentialType = "antigravity"): Promise<boolean> {
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
