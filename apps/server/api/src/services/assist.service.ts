/**
 * Desktop assistant support service: FFF-backed fuzzy file search for the
 * @-mention picker. Uses the same Rust-native FileFinder that powers the
 * grep/glob agent tools, so repeat searches reuse a warm index.
 */
import { FileFinder } from "@ff-labs/fff-node";
import * as path from "node:path";
import { existsSync } from "node:fs";
import type { FileSearchResult } from "@console/types";

const MAX_RESULTS = 20;

/**
 * Expand `@path/to/file` mentions (relative to the session cwd) into absolute
 * paths before the prompt reaches the agent. This is the server-side half of
 * the desktop @-mention picker: the UI inserts a relative ref, and here it is
 * resolved against the working directory so the agent's read tools can load it.
 */
export function expandPromptRefs(prompt: string, cwd: string): string {
  const base = path.resolve(cwd);
  return prompt.replace(/(^|\s)@([^\s@]+)/g, (_match, prefix: string, raw: string) => {
    // Skip email-ish or already-absolute references.
    if (path.isAbsolute(raw) || raw.includes("@")) {
      return `${prefix}@${raw}`;
    }
    const resolved = path.resolve(base, raw);
    if (existsSync(resolved)) {
      return `${prefix}${resolved}`;
    }
    return `${prefix}@${raw}`;
  });
}

export async function searchFiles(root: string, query: string): Promise<FileSearchResult[]> {
  const basePath = path.resolve(root);

  const created = FileFinder.create({ basePath });
  if (!created.ok) {
    throw new Error(`Failed to initialise file finder: ${created.error}`);
  }

  const finder = created.value;
  try {
    await finder.waitForScan(5000);

    const result = finder.mixedSearch(query, { pageSize: MAX_RESULTS });
    if (!result.ok) {
      throw new Error(`Search error: ${result.error}`);
    }

    const { items, scores } = result.value;
    return items.map((entry, i) => {
      const isDir = entry.type === "directory";
      const relativePath = entry.item.relativePath;
      return {
        relativePath,
        absolutePath: path.join(basePath, relativePath),
        isDir,
        score: scores[i]?.total ?? 0,
      };
    });
  } finally {
    finder.destroy();
  }
}
