/**
 * Discover user-defined slash commands from `commands/*.md`.
 * Execution lands in Phase 6; discovery only for inventory / future registry.
 */
import * as path from "node:path";
import type { SlashCommandFile } from "../types/system-prompt.js";
import { parseFrontmatter } from "./frontmatter.js";
import {
  createSourceMeta,
  getProjectConfigDirs,
  getUserConfigDirs,
  listMarkdownFiles,
  readTextFile,
  resolveRoots,
} from "./walk.js";

function commandNameFromPath(filePath: string, frontmatterName?: string): string {
  if (frontmatterName?.trim()) return frontmatterName.trim().replace(/^\//, "");
  return path.basename(filePath).replace(/\.(mdc|md|markdown)$/i, "");
}

async function loadCommandFile(
  filePath: string,
  level: "user" | "project",
): Promise<SlashCommandFile | null> {
  const raw = await readTextFile(filePath);
  if (raw === null || !raw.trim()) return null;

  const { frontmatter, body } = parseFrontmatter(raw);
  const name = commandNameFromPath(
    filePath,
    typeof frontmatter.name === "string" ? frontmatter.name : undefined,
  );

  return {
    name,
    path: path.resolve(filePath),
    content: body.trim(),
    description: typeof frontmatter.description === "string" ? frontmatter.description : undefined,
    level,
    source: createSourceMeta("commands", filePath, level),
  };
}

async function loadCommandsFromDir(
  dir: string,
  level: "user" | "project",
): Promise<SlashCommandFile[]> {
  const files = await listMarkdownFiles(path.join(dir, "commands"));
  const commands: SlashCommandFile[] = [];
  for (const file of files) {
    const cmd = await loadCommandFile(file, level);
    if (cmd) commands.push(cmd);
  }
  return commands;
}

export async function discoverCommands(
  options: { cwd?: string; home?: string; stopAt?: string } = {},
): Promise<SlashCommandFile[]> {
  const roots = resolveRoots(options);
  const byName = new Map<string, SlashCommandFile>();

  for (const userDir of await getUserConfigDirs(roots.home)) {
    for (const cmd of await loadCommandsFromDir(userDir, "user")) {
      byName.set(cmd.name, cmd);
    }
  }

  const projectDirs = await getProjectConfigDirs(roots.cwd, roots.stopAt);
  const ordered = [...projectDirs].sort((a, b) => b.depth - a.depth);
  for (const { dir } of ordered) {
    for (const cmd of await loadCommandsFromDir(dir, "project")) {
      byName.set(cmd.name, cmd);
    }
  }

  return Array.from(byName.values()).sort((a, b) => a.name.localeCompare(b.name));
}
