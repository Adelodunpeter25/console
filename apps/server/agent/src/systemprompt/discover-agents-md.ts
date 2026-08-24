/**
 * Discover AGENTS.md / context instruction files.
 * Inspired by oh-my-pi `discovery/agents-md.ts` + multi-provider context files.
 */
import * as path from "node:path";
import type { ContextFile } from "@/agent/src/types/system-prompt.js";
import {
  createSourceMeta,
  getAncestorDirs,
  getProjectConfigDirs,
  getUserConfigDirs,
  readTextFile,
  resolveRoots,
  type DiscoveryRoots,
} from "./walk.js";

/** Standalone filenames treated as context files when found at ancestor roots. */
const ROOT_CONTEXT_NAMES = [
  "AGENTS.md",
  "CLAUDE.md",
  "GEMINI.md",
  "CODEX.md",
  ".cursorrules",
] as const;

/** Context filenames inside config dirs (.agent/, .console/, …). */
const CONFIG_CONTEXT_NAMES = ["AGENTS.md", "CLAUDE.md", "GEMINI.md"] as const;

function dedupeByContent(files: ContextFile[]): ContextFile[] {
  const lastIndexByContent = new Map<string, number>();
  for (let i = 0; i < files.length; i++) {
    lastIndexByContent.set(files[i]!.content, i);
  }
  return files.filter((file, index) => lastIndexByContent.get(file.content) === index);
}

async function tryPushContext(
  items: ContextFile[],
  filePath: string,
  level: "user" | "project",
  depth: number | undefined,
  provider: string,
): Promise<void> {
  const content = await readTextFile(filePath);
  if (content === null || !content.trim()) return;

  // Skip AGENTS.md living inside a hidden parent that is not a known config dir
  // (matches oh-my-pi agents-md: ignore e.g. `.git/AGENTS.md`)
  if (level === "project") {
    const parentBase = path.basename(path.dirname(filePath));
    const isConfigDir = parentBase.startsWith(".");
    if (isConfigDir) {
      const allowed = new Set([
        ".console",
        ".agent",
        ".agents",
        ".claude",
        ".gemini",
        ".codex",
        ".cursor",
      ]);
      if (!allowed.has(parentBase) && CONFIG_CONTEXT_NAMES.every((n) => !filePath.endsWith(n))) {
        // still allow known config dir names only
      }
    }
  }

  items.push({
    path: path.resolve(filePath),
    content,
    level,
    depth,
    source: createSourceMeta(provider, filePath, level),
  });
}

/**
 * Load project + user context files.
 * Sorted so farther files come first and cwd-closer files are more prominent last.
 */
export async function discoverContextFiles(
  options: { cwd?: string; home?: string; stopAt?: string } = {},
): Promise<ContextFile[]> {
  const roots: DiscoveryRoots = resolveRoots(options);
  const items: ContextFile[] = [];

  // 1. Standalone root-level files walking up from cwd
  for (const { dir, depth } of getAncestorDirs(roots.cwd, roots.stopAt)) {
    for (const name of ROOT_CONTEXT_NAMES) {
      const candidate = path.join(dir, name);
      // Skip files whose immediate parent is a hidden dir (except we already join at dir root)
      const parentBase = path.basename(dir);
      if (parentBase.startsWith(".") && parentBase !== "." && parentBase !== "..") {
        continue;
      }
      await tryPushContext(items, candidate, "project", depth, "agents-md");
    }
  }

  // 2. Config-dir context files at each ancestor (.agent/AGENTS.md, …)
  for (const { dir, depth } of await getProjectConfigDirs(roots.cwd, roots.stopAt)) {
    for (const name of CONFIG_CONTEXT_NAMES) {
      await tryPushContext(items, path.join(dir, name), "project", depth, "agents-md-config");
    }
  }

  // 3. User-level
  for (const userDir of await getUserConfigDirs(roots.home)) {
    for (const name of CONFIG_CONTEXT_NAMES) {
      await tryPushContext(items, path.join(userDir, name), "user", undefined, "agents-md-user");
    }
  }

  // Sort: higher depth first (farther), then user last among ties → closer project last/prominent
  items.sort((a, b) => {
    const depthA = a.depth ?? -1;
    const depthB = b.depth ?? -1;
    if (depthA !== depthB) return depthB - depthA;
    if (a.level !== b.level) return a.level === "user" ? -1 : 1;
    return a.path.localeCompare(b.path);
  });

  return dedupeByContent(items);
}
