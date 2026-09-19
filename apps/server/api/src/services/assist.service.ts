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

/** Reused FileFinder per project root so repeat searches hit a warm index. */
interface CachedFinder {
  finder: FileFinder;
  lastUsed: number;
  warm: boolean;
}
const finderCache = new Map<string, CachedFinder>();
const MAX_CACHED_FINDERS = 8;
const FINDER_IDLE_MS = 5 * 60 * 1000;

function evictStaleFinders(now: number): void {
  for (const [key, entry] of finderCache) {
    if (now - entry.lastUsed > FINDER_IDLE_MS || entry.finder.isDestroyed) {
      try {
        if (!entry.finder.isDestroyed) entry.finder.destroy();
      } catch {
        // Best-effort cleanup.
      }
      finderCache.delete(key);
    }
  }
  while (finderCache.size > MAX_CACHED_FINDERS) {
    let oldestKey: string | null = null;
    let oldestTime = Number.POSITIVE_INFINITY;
    for (const [key, entry] of finderCache) {
      if (entry.lastUsed < oldestTime) {
        oldestTime = entry.lastUsed;
        oldestKey = key;
      }
    }
    if (oldestKey === null) break;
    const victim = finderCache.get(oldestKey);
    try {
      if (victim && !victim.finder.isDestroyed) victim.finder.destroy();
    } catch {
      // Best-effort cleanup.
    }
    finderCache.delete(oldestKey);
  }
}

function getCachedFinder(basePath: string): { finder: FileFinder; warm: boolean } {
  const now = Date.now();
  evictStaleFinders(now);
  const hit = finderCache.get(basePath);
  if (hit && !hit.finder.isDestroyed) {
    hit.lastUsed = now;
    return { finder: hit.finder, warm: hit.warm };
  }
  if (hit) finderCache.delete(basePath);
  const created = FileFinder.create({ basePath });
  if (!created.ok) {
    throw new Error(`Failed to initialise file finder: ${created.error}`);
  }
  finderCache.set(basePath, { finder: created.value, lastUsed: now, warm: false });
  return { finder: created.value, warm: false };
}

function markFinderWarm(basePath: string): void {
  const entry = finderCache.get(basePath);
  if (entry) {
    entry.warm = true;
    entry.lastUsed = Date.now();
  }
}

function dropFinder(basePath: string): void {
  const entry = finderCache.get(basePath);
  if (entry) {
    try {
      if (!entry.finder.isDestroyed) entry.finder.destroy();
    } catch {
      // Best-effort cleanup.
    }
    finderCache.delete(basePath);
  }
}

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

  const { finder, warm } = getCachedFinder(basePath);
  try {
    // Cold index waits up to 5s; warm reuse only needs a short grace period
    // since the background scan is already running.
    await finder.waitForScan(warm ? 500 : 5000);
    markFinderWarm(basePath);

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
  } catch (err) {
    // A broken native handle must not poison the cache for later searches.
    dropFinder(basePath);
    throw err;
  }
}
