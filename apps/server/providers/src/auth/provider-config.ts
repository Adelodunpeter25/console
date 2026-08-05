/**
 * Per-provider configuration storage (non-secret settings like the
 * configured Google Cloud project ID for Gemini).
 *
 * Stored at ~/.console/{provider}-config.json so it persists across
 * restarts and is available before the user has logged in.
 */
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";

export type ProviderType = "gemini" | "antigravity";

interface ProviderConfig {
  /** Explicit Google Cloud project ID to use during login. */
  configuredProjectId?: string;
}

function configFilePath(type: ProviderType): string {
  return path.join(os.homedir(), ".console", `${type}-config.json`);
}

export async function getProviderConfig(type: ProviderType): Promise<ProviderConfig> {
  try {
    const content = await fs.readFile(configFilePath(type), "utf-8");
    return JSON.parse(content) as ProviderConfig;
  } catch {
    return {};
  }
}

export async function getConfiguredProjectId(type: ProviderType): Promise<string | undefined> {
  const config = await getProviderConfig(type);
  const id = config.configuredProjectId?.trim();
  return id || undefined;
}

export async function setConfiguredProjectId(
  type: ProviderType,
  projectId: string | undefined,
): Promise<void> {
  const filePath = configFilePath(type);
  const existing = await getProviderConfig(type);
  const trimmed = projectId?.trim() || undefined;

  const updated: ProviderConfig = {
    ...existing,
    configuredProjectId: trimmed,
  };

  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(updated, null, 2), "utf-8");
}
