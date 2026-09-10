import * as fs from "node:fs/promises";
import * as path from "node:path";
import { getConsoleStorageDir } from "@/agent/src/session/apppaths.js";
import { findModelInProvider, getProvider } from "@/agent/src/commands/provider-registry.js";
import type { Model, ProviderId } from "@console/types";

export type ConsoleModelRole = "default" | "plan" | "vision" | "smol";
export type ModelRoleMapping = Partial<Record<ConsoleModelRole, string>>;

export interface ConsoleSettings {
  modelRoles?: ModelRoleMapping;
}

const SETTINGS_FILE = "settings.json";
const ROLES: ConsoleModelRole[] = ["default", "plan", "vision", "smol"];

export function getSettingsPath(): string {
  return path.join(getConsoleStorageDir(), SETTINGS_FILE);
}

export function defaultSettings(): ConsoleSettings {
  return { modelRoles: {} };
}

export async function loadSettings(): Promise<ConsoleSettings> {
  try {
    const parsed = JSON.parse(await fs.readFile(getSettingsPath(), "utf8")) as ConsoleSettings;
    const roles = parsed.modelRoles && typeof parsed.modelRoles === "object" ? parsed.modelRoles : {};
    return { modelRoles: sanitizeRoles(roles) };
  } catch {
    return defaultSettings();
  }
}

export async function saveSettings(settings: ConsoleSettings): Promise<ConsoleSettings> {
  const normalized = { modelRoles: sanitizeRoles(settings.modelRoles ?? {}) };
  const file = getSettingsPath();
  await fs.mkdir(path.dirname(file), { recursive: true });
  await fs.writeFile(file, `${JSON.stringify(normalized, null, 2)}\n`, "utf8");
  return normalized;
}

function sanitizeRoles(roles: ModelRoleMapping): ModelRoleMapping {
  const result: ModelRoleMapping = {};
  for (const role of ROLES) {
    const value = roles[role];
    if (typeof value === "string" && value.trim()) result[role] = value.trim();
  }
  return result;
}

function parseModelReference(reference: string, fallback: Model): { provider: ProviderId; modelId: string } {
  const slash = reference.indexOf("/");
  if (slash > 0) return { provider: reference.slice(0, slash) as ProviderId, modelId: reference.slice(slash + 1) };
  return { provider: fallback.provider, modelId: reference };
}

export async function resolveRoleModel(role: ConsoleModelRole, fallback: Model): Promise<Model> {
  const settings = await loadSettings();
  const reference = settings.modelRoles?.[role] ?? settings.modelRoles?.default;
  if (!reference) return fallback;
  const parsed = parseModelReference(reference, fallback);
  if (!getProvider(parsed.provider)) return fallback;
  return findModelInProvider(parsed.provider, parsed.modelId) ?? {
    id: parsed.modelId,
    provider: parsed.provider,
    contextWindow: 128_000,
  };
}

export function isConsoleModelRole(value: unknown): value is ConsoleModelRole {
  return typeof value === "string" && ROLES.includes(value as ConsoleModelRole);
}
