/**
 * Cline API key storage. No OAuth — single static key.
 * Stored at ~/.console/cline-creds.json
 * Overridable via CLINE_CREDENTIALS_PATH env var.
 *
 * Lookup precedence (first non-empty wins):
 *   1. CLINE_API_KEY env var
 *   2. ~/.console/cline-creds.json (CLINE_CREDENTIALS_PATH overrides path)
 *   3. null
 */
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";

export interface ClineCredential {
  apiKey: string;
}

function credentialFilePath(): string {
  return (
    process.env.CLINE_CREDENTIALS_PATH ??
    path.join(os.homedir(), ".console", "cline-creds.json")
  );
}

export async function loadClineCredential(): Promise<ClineCredential | null> {
  const envKey = process.env.CLINE_API_KEY?.trim();
  if (envKey) return { apiKey: envKey };

  try {
    const raw = await fs.readFile(credentialFilePath(), "utf-8");
    const parsed = JSON.parse(raw) as Partial<ClineCredential>;
    if (typeof parsed.apiKey === "string" && parsed.apiKey.length > 0) {
      return { apiKey: parsed.apiKey };
    }
    return null;
  } catch {
    return null;
  }
}

export async function saveClineCredential(cred: ClineCredential): Promise<void> {
  const filePath = credentialFilePath();
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(cred, null, 2), "utf-8");
}

export async function clearClineCredential(): Promise<void> {
  try {
    await fs.unlink(credentialFilePath());
  } catch {
    // Already gone
  }
}