/**
 * Discover SYSTEM.md overrides.
 * Project-level SYSTEM.md wins over user-level (oh-my-pi loadSystemPromptFiles).
 */
import * as path from "node:path";
import type { SystemPromptFile } from "@/agent/src/types/system-prompt.js";
import {
  createSourceMeta,
  getProjectConfigDirs,
  getUserConfigDirs,
  readTextFile,
  resolveRoots,
} from "./walk.js";

/**
 * Load the effective SYSTEM.md customization.
 * Returns project if present, otherwise user, otherwise null.
 */
export async function discoverSystemPromptFile(
  options: { cwd?: string; home?: string; stopAt?: string } = {},
): Promise<SystemPromptFile | null> {
  const roots = resolveRoots(options);

  // Nearest project SYSTEM.md (closest to cwd first in getProjectConfigDirs)
  const projectDirs = await getProjectConfigDirs(roots.cwd, roots.stopAt);
  // Prefer closest depth
  const byDepth = [...projectDirs].sort((a, b) => a.depth - b.depth);
  for (const { dir } of byDepth) {
    const filePath = path.join(dir, "SYSTEM.md");
    const content = await readTextFile(filePath);
    if (content !== null && content.trim()) {
      return {
        path: path.resolve(filePath),
        content,
        level: "project",
        source: createSourceMeta("system-md", filePath, "project"),
      };
    }
  }

  for (const userDir of await getUserConfigDirs(roots.home)) {
    const filePath = path.join(userDir, "SYSTEM.md");
    const content = await readTextFile(filePath);
    if (content !== null && content.trim()) {
      return {
        path: path.resolve(filePath),
        content,
        level: "user",
        source: createSourceMeta("system-md", filePath, "user"),
      };
    }
  }

  return null;
}
