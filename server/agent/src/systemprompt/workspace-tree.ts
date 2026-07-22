/**
 * Lightweight workspace tree for the system prompt.
 * Pure Node fs — no native deps. Inspired by oh-my-pi workspace-tree defaults.
 */
import * as fs from "node:fs/promises";
import * as path from "node:path";
import type { WorkspaceTree } from "../types/system-prompt.js";

const DEFAULTS = {
  maxDepth: 3,
  perDirLimit: 12,
  lineCap: 120,
} as const;

const SKIP_DIRS = new Set([
  "node_modules",
  ".git",
  ".hg",
  ".svn",
  "dist",
  "build",
  "out",
  "coverage",
  ".next",
  ".turbo",
  ".cache",
  "__pycache__",
  ".venv",
  "venv",
  "target",
]);

export interface BuildWorkspaceTreeOptions {
  maxDepth?: number;
  perDirLimit?: number;
  lineCap?: number;
  showHidden?: boolean;
}

interface TreeEntry {
  name: string;
  isDir: boolean;
  children?: TreeEntry[];
}

async function readChildren(
  dirPath: string,
  depth: number,
  maxDepth: number,
  perDirLimit: number,
  showHidden: boolean,
): Promise<{ entries: TreeEntry[]; truncated: boolean }> {
  let dirents;
  try {
    dirents = await fs.readdir(dirPath, { withFileTypes: true });
  } catch {
    return { entries: [], truncated: false };
  }

  const filtered = dirents.filter((d) => {
    if (!showHidden && d.name.startsWith(".")) return false;
    if (d.isDirectory() && SKIP_DIRS.has(d.name)) return false;
    return true;
  });

  // Dirs first, then files; alpha within each group
  filtered.sort((a, b) => {
    if (a.isDirectory() !== b.isDirectory()) return a.isDirectory() ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  let truncated = false;
  let slice = filtered;
  if (filtered.length > perDirLimit) {
    slice = filtered.slice(0, perDirLimit);
    truncated = true;
  }

  const entries: TreeEntry[] = [];
  for (const d of slice) {
    if (d.isDirectory()) {
      const child: TreeEntry = { name: d.name, isDir: true };
      if (depth < maxDepth) {
        const nested = await readChildren(
          path.join(dirPath, d.name),
          depth + 1,
          maxDepth,
          perDirLimit,
          showHidden,
        );
        child.children = nested.entries;
        truncated = truncated || nested.truncated;
      }
      entries.push(child);
    } else {
      entries.push({ name: d.name, isDir: false });
    }
  }

  return { entries, truncated };
}

function renderTree(
  entries: TreeEntry[],
  prefix: string,
  lines: string[],
  lineCap: number,
): boolean {
  for (let i = 0; i < entries.length; i++) {
    if (lines.length >= lineCap) return true;
    const entry = entries[i]!;
    const isLast = i === entries.length - 1;
    const connector = isLast ? "└── " : "├── ";
    const childPrefix = prefix + (isLast ? "    " : "│   ");
    lines.push(`${prefix}${connector}${entry.name}${entry.isDir ? "/" : ""}`);
    if (entry.children && entry.children.length > 0) {
      const hitCap = renderTree(entry.children, childPrefix, lines, lineCap);
      if (hitCap) return true;
    }
  }
  return false;
}

export async function buildWorkspaceTree(
  cwd: string,
  options: BuildWorkspaceTreeOptions = {},
): Promise<WorkspaceTree> {
  const rootPath = path.resolve(cwd);
  const maxDepth = options.maxDepth ?? DEFAULTS.maxDepth;
  const perDirLimit = options.perDirLimit ?? DEFAULTS.perDirLimit;
  const lineCap = options.lineCap ?? DEFAULTS.lineCap;
  const showHidden = options.showHidden ?? false;

  const { entries, truncated: dirTruncated } = await readChildren(
    rootPath,
    1,
    maxDepth,
    perDirLimit,
    showHidden,
  );

  const lines: string[] = [path.basename(rootPath) + "/"];
  const lineTruncated = renderTree(entries, "", lines, lineCap);
  const truncated = dirTruncated || lineTruncated;

  if (truncated) {
    lines.push("… (tree truncated — use listDir/glob to drill in)");
  }

  return {
    rootPath,
    rendered: lines.join("\n"),
    truncated,
    totalLines: lines.length,
  };
}
