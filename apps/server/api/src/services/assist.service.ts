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

export async function searchFiles(
  root: string,
  query: string,
  maxResults = MAX_RESULTS,
  // When false, directories are filtered out so every slot holds a file.
  // Defaults to true to preserve the file-browser and @-mention behavior.
  includeDirs = true,
): Promise<FileSearchResult[]> {
  const basePath = path.resolve(root);

  const created = FileFinder.create({ basePath });
  if (!created.ok) {
    throw new Error(`Failed to initialise file finder: ${created.error}`);
  }

  const finder = created.value;
  try {
    await finder.waitForScan(5000);

    // Request headroom when excluding directories so a full page of files
    // survives the filter instead of returning short.
    const result = finder.mixedSearch(query, {
      pageSize: includeDirs ? maxResults : maxResults * 3,
    });
    if (!result.ok) {
      throw new Error(`Search error: ${result.error}`);
    }

    const { items, scores } = result.value;
    return items
      .map((entry, i) => ({ entry, score: scores[i]?.total ?? 0 }))
      .filter(({ entry }) => includeDirs || entry.type !== "directory")
      .slice(0, maxResults)
      .map(({ entry, score }) => {
        const relativePath = entry.item.relativePath;
        return {
          relativePath,
          absolutePath: path.join(basePath, relativePath),
          isDir: entry.type === "directory",
          score,
        };
      });
  } finally {
    finder.destroy();
  }
}
