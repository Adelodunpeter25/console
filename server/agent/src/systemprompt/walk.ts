/**
 * Filesystem walk-up helpers for config discovery.
 * Mirrors oh-my-pi discovery ancestor walks without the capability registry.
 */
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import type { ConfigLevel, SourceMeta } from "../types/system-prompt.js";

/** Project-level config directory names (relative to each ancestor). */
export const PROJECT_CONFIG_DIRS = [".console", ".agent", ".agents"] as const;

/** User-level config directory names under home. */
export const USER_CONFIG_DIRS = [".console", ".agent", ".agents"] as const;

export interface DiscoveryRoots {
  cwd: string;
  home: string;
  /** Optional stop path for project walk (repo root). */
  stopAt?: string;
}

export function resolveRoots(options: {
  cwd?: string;
  home?: string;
  stopAt?: string;
}): DiscoveryRoots {
  return {
    cwd: path.resolve(options.cwd ?? process.cwd()),
    home: options.home ?? os.homedir(),
    stopAt: options.stopAt ? path.resolve(options.stopAt) : undefined,
  };
}

export function createSourceMeta(
  provider: string,
  filePath: string,
  level: ConfigLevel,
): SourceMeta {
  return {
    provider,
    path: path.resolve(filePath),
    level,
  };
}

/** Ancestors from cwd up to stopAt / filesystem root (cwd first). */
export function getAncestorDirs(
  cwd: string,
  stopAt?: string,
): Array<{ dir: string; depth: number }> {
  const ancestors: Array<{ dir: string; depth: number }> = [];
  let current = path.resolve(cwd);
  let depth = 0;
  while (true) {
    ancestors.push({ dir: current, depth });
    if (stopAt && current === path.resolve(stopAt)) break;
    const parent = path.dirname(current);
    if (parent === current) break;
    current = parent;
    depth++;
  }
  return ancestors;
}

export async function readTextFile(filePath: string): Promise<string | null> {
  try {
    return await fs.readFile(filePath, "utf8");
  } catch (err: unknown) {
    const code = (err as NodeJS.ErrnoException).code;
    if (code === "ENOENT" || code === "ENOTDIR") return null;
    throw err;
  }
}

export async function pathExists(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

export async function isDirectory(dirPath: string): Promise<boolean> {
  try {
    const stat = await fs.stat(dirPath);
    return stat.isDirectory();
  } catch {
    return false;
  }
}

/** List markdown-like files in a directory (non-recursive). */
export async function listMarkdownFiles(dirPath: string): Promise<string[]> {
  if (!(await isDirectory(dirPath))) return [];
  const entries = await fs.readdir(dirPath, { withFileTypes: true });
  return entries
    .filter(
      (e) =>
        e.isFile() &&
        (e.name.endsWith(".md") || e.name.endsWith(".mdc") || e.name.endsWith(".markdown")),
    )
    .map((e) => path.join(dirPath, e.name))
    .sort();
}

/** User config base dirs that exist under home. */
export async function getUserConfigDirs(home: string): Promise<string[]> {
  const dirs: string[] = [];
  for (const name of USER_CONFIG_DIRS) {
    const dir = path.join(home, name);
    if (await isDirectory(dir)) dirs.push(dir);
  }
  return dirs;
}

/**
 * Project config dirs for each ancestor: `<ancestor>/.console`, `.agent`, `.agents`.
 * Closest to cwd first.
 */
export async function getProjectConfigDirs(
  cwd: string,
  stopAt?: string,
): Promise<Array<{ dir: string; depth: number }>> {
  const result: Array<{ dir: string; depth: number }> = [];
  for (const { dir: ancestor, depth } of getAncestorDirs(cwd, stopAt)) {
    for (const name of PROJECT_CONFIG_DIRS) {
      const configDir = path.join(ancestor, name);
      if (await isDirectory(configDir)) {
        result.push({ dir: configDir, depth });
      }
    }
  }
  return result;
}

export function depthBetween(from: string, to: string): number {
  const fromParts = path.resolve(from).split(path.sep).filter(Boolean);
  const toParts = path.resolve(to).split(path.sep).filter(Boolean);
  // if `to` is ancestor of `from`, depth is difference in segments
  if (toParts.length > fromParts.length) return Math.abs(toParts.length - fromParts.length);
  let i = 0;
  while (i < toParts.length && i < fromParts.length && toParts[i] === fromParts[i]) i++;
  return fromParts.length - i;
}
