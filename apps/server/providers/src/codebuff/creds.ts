/**
 * Codebuff credential store — reads/writes the auth token obtained from the
 * device-code login flow (same credentials file format as the official CLI's
 * `credentials.json`, but stored under ~/.console/codebuff-creds.json).
 *
 * Resolution order (matches the CLI's getAuthTokenDetails):
 *   1. CODEBUFF_API_KEY environment variable
 *   2. ~/.console/codebuff-creds.json (or CODEBUFF_CREDENTIALS_PATH override)
 */
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";

import {
  CODEBUFF_API_KEY_ENV_VAR,
  CODEBUFF_CREDENTIALS_FILE,
} from "./constants.js";

export interface CodebuffCredential {
  authToken: string;
  id?: string;
  name?: string;
  email?: string;
}

export function getCodebuffCredentialsPath(): string {
  return (
    process.env.CODEBUFF_CREDENTIALS_PATH ??
    path.join(os.homedir(), ".console", CODEBUFF_CREDENTIALS_FILE)
  );
}

/**
 * Load the Codebuff auth token. Env var takes precedence over the
 * credential file (same order as the official CLI).
 */
export async function loadCodebuffCredential(): Promise<CodebuffCredential | null> {
  const envToken = process.env[CODEBUFF_API_KEY_ENV_VAR];
  if (envToken) {
    return { authToken: envToken };
  }

  const filePath = getCodebuffCredentialsPath();
  try {
    const raw = JSON.parse(
      await fs.readFile(filePath, "utf-8"),
    ) as Partial<CodebuffCredential>;
    if (typeof raw.authToken === "string" && raw.authToken.length > 0) {
      return raw as CodebuffCredential;
    }
    return null;
  } catch {
    return null;
  }
}

export async function saveCodebuffCredential(
  cred: CodebuffCredential,
): Promise<void> {
  const filePath = getCodebuffCredentialsPath();
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(cred, null, 2), "utf-8");
}

export async function clearCodebuffCredential(): Promise<void> {
  const filePath = getCodebuffCredentialsPath();
  try {
    await fs.unlink(filePath);
  } catch {
    // Already gone — nothing to do.
  }
}

export async function hasCodebuffCredential(): Promise<boolean> {
  return (await loadCodebuffCredential()) !== null;
}
