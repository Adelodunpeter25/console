import fs from "node:fs/promises";
import path from "node:path";
import TOML from "@iarna/toml";
import type { ProjectScript, ProjectScriptsResult } from "./types.js";

const SCRIPT_ID = /^[A-Za-z0-9_-]+$/;
const SHORTCUT = /^(cmd|ctrl|alt|shift)(-(cmd|ctrl|alt|shift))*-[a-z0-9]+$/i;
const MAX_LABEL_LENGTH = 200;
const MAX_COMMAND_LENGTH = 4096;

function fail(message: string): never {
  throw new Error(`Invalid console.toml: ${message}`);
}

function record(value: unknown, name: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) fail(`${name} must be a table.`);
  return value as Record<string, unknown>;
}

function stringField(value: unknown, name: string, max: number): string {
  if (typeof value !== "string" || !value.trim()) fail(`${name} must be a non-empty string.`);
  if (value.length > max) fail(`${name} is too long.`);
  return value;
}

export function parseProjectScripts(text: string): ProjectScript[] {
  let parsed: unknown;
  try {
    parsed = TOML.parse(text);
  } catch (error) {
    fail(error instanceof Error ? error.message : String(error));
  }
  const root = record(parsed, "root");
  if (root.scripts === undefined) return [];
  const scripts = record(root.scripts, "scripts");
  return Object.entries(scripts).map(([id, raw]) => {
    if (!SCRIPT_ID.test(id)) fail(`script identifier '${id}' contains unsupported characters.`);
    const entry = record(raw, `scripts.${id}`);
    const allowed = new Set(["label", "command", "shortcut", "persistent"]);
    for (const key of Object.keys(entry)) if (!allowed.has(key)) fail(`scripts.${id}.${key} is not supported.`);
    const label = stringField(entry.label, `scripts.${id}.label`, MAX_LABEL_LENGTH);
    const command = stringField(entry.command, `scripts.${id}.command`, MAX_COMMAND_LENGTH);
    let shortcut: string | null = null;
    if (entry.shortcut !== undefined) {
      shortcut = stringField(entry.shortcut, `scripts.${id}.shortcut`, 100);
      if (!SHORTCUT.test(shortcut)) fail(`scripts.${id}.shortcut has unsupported syntax.`);
    }
    if (entry.persistent !== undefined && typeof entry.persistent !== "boolean") {
      fail(`scripts.${id}.persistent must be a boolean.`);
    }
    return { id, label, command, shortcut, persistent: entry.persistent === true };
  });
}

export async function loadProjectScripts(projectId: string, projectRoot: string): Promise<ProjectScriptsResult> {
  const filePath = path.join(projectRoot, "console.toml");
  try {
    const text = await fs.readFile(filePath, "utf8");
    return { projectId, scripts: parseProjectScripts(text), source: "console.toml" };
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return { projectId, scripts: [], source: "missing" };
    throw error;
  }
}
