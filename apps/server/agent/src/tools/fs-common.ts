import * as fs from "node:fs/promises";
import * as path from "node:path";

/** A name clients should never show (dotfiles) unless explicitly requested. */
export function isHiddenName(name: string): boolean {
  return name.startsWith(".");
}

/**
 * Best-effort file size for directory listings.
 * Returns `undefined` when the entry is a directory or stat fails (races,
 * permissions) so the listing never fails for one unreadable entry.
 */
export async function safeFileSize(entryPath: string, isDir: boolean): Promise<number | undefined> {
  if (isDir) return undefined;
  try {
    const stat = await fs.stat(entryPath);
    return stat.size;
  } catch {
    return undefined;
  }
}

/** Sort directories first, then files alphabetically (numeric-aware). */
export function sortDirsFirst<T extends { name: string; isDir: boolean }>(entries: T[]): T[] {
  return entries.sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    return a.name.localeCompare(b.name, undefined, { numeric: true });
  });
}

/**
 * Normalize a possibly-filtered directory listing into sorted `FsTreeEntry`s.
 * Shared by `FsService.browseDirectory` so the tool and the API route agree
 * on ordering and hidden-file behavior.
 */
export async function toSortedEntries(
  dirPath: string,
  dirents: Array<{ name: string; isDirectory: () => boolean }>,
  options?: { showHidden?: boolean },
): Promise<Array<{ name: string; path: string; isDir: boolean; size?: number }>> {
  const showHidden = options?.showHidden ?? false;
  const visible = dirents.filter((e) => showHidden || !isHiddenName(e.name));
  const pending = visible.map(async (entry) => {
    const entryPath = path.join(dirPath, entry.name);
    const isDir = entry.isDirectory();
    return {
      name: entry.name,
      path: entryPath,
      isDir,
      size: await safeFileSize(entryPath, isDir),
    };
  });
  const entries = await Promise.all(pending);
  return sortDirsFirst(entries);
}
