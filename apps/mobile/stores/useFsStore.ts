import { batch, observable } from "@legendapp/state";
import type { FsTreeEntry } from "@console/types";
import { fsService } from "@console/api";
import type { BrowseResult, DirectoryTreeResult, ReadFileResult } from "@/types";

/**
 * Filesystem browse/cache state as Legend State observables.
 * See docs/legend-state-and-list-migration.md.
 *
 * Reads in components subscribe via `useValue(fs$.field)`;
 * imperative reads outside render use `.peek()`.
 */
export const fs$ = observable({
  /** Current browse location and its entries (file-picker navigation). */
  browse: null as BrowseResult | null,
  browsing: false,
  /** Cached directory trees keyed by path. */
  treesByPath: {} as Record<string, DirectoryTreeResult>,
  /** Cached file contents keyed by path (line-range-agnostic). */
  fileContentsByPath: {} as Record<string, ReadFileResult>,
  /** Set of paths with an in-flight operation. */
  busyPaths: {} as Record<string, boolean>,
  error: null as string | null,
});

function markBusy(path: string): void {
  fs$.busyPaths[path].set(true);
}

function clearBusy(path: string): void {
  fs$.busyPaths[path].delete();
}

function invalidateFileContents(path: string): void {
  fs$.fileContentsByPath[path].delete();
}

export async function browseDirectory(path?: string): Promise<BrowseResult> {
  batch(() => {
    fs$.browsing.set(true);
    fs$.error.set(null);
  });
  try {
    const result = await fsService.getFsBrowse(path);
    batch(() => {
      fs$.browse.set(result);
      fs$.browsing.set(false);
    });
    return result;
  } catch (e) {
    batch(() => {
      fs$.browsing.set(false);
      fs$.error.set(e instanceof Error ? e.message : "Failed to browse directory");
    });
    throw e;
  }
}

export async function getDirectoryTree(path?: string, depth?: number): Promise<DirectoryTreeResult> {
  const key = path ?? "";
  const cached = fs$.treesByPath[key].peek();
  if (cached) return cached;

  fs$.error.set(null);
  const result = await fsService.getFsTree(path);
  // Convert the flat tree list into the desktop-shaped tree result.
  const treeResult: DirectoryTreeResult = {
    path: path ?? "",
    treeFormatted: result
      .map((entry: FsTreeEntry) => formatTreeEntry(entry))
      .join("\n"),
  };
  fs$.treesByPath[key].set(treeResult);
  return treeResult;
}

export async function readFile(
  path: string,
  startLine?: number,
  endLine?: number,
): Promise<ReadFileResult> {
  const cached = fs$.fileContentsByPath[path].peek();
  if (cached && !startLine && !endLine) return cached;

  fs$.error.set(null);
  const result = await fsService.readFile(path);
  const readResult: ReadFileResult = {
    path: result.path,
    content: result.content,
  };
  if (!startLine && !endLine) {
    fs$.fileContentsByPath[path].set(readResult);
  }
  return readResult;
}

export async function writeFile(path: string, content: string): Promise<void> {
  markBusy(path);
  fs$.error.set(null);
  try {
    await fsService.writeFile(path, content);
    // Invalidate cached content so the next read fetches the new state.
    invalidateFileContents(path);
  } catch (e) {
    fs$.error.set(e instanceof Error ? e.message : "Failed to write file");
    throw e;
  } finally {
    clearBusy(path);
  }
}

export async function deleteFile(path: string): Promise<void> {
  markBusy(path);
  fs$.error.set(null);
  try {
    await fsService.deleteFile(path);
    invalidateFileContents(path);
  } catch (e) {
    fs$.error.set(e instanceof Error ? e.message : "Failed to delete file");
    throw e;
  } finally {
    clearBusy(path);
  }
}

export async function createDirectory(path: string): Promise<void> {
  markBusy(path);
  fs$.error.set(null);
  try {
    await fsService.createDir(path);
  } catch (e) {
    fs$.error.set(e instanceof Error ? e.message : "Failed to create directory");
    throw e;
  } finally {
    clearBusy(path);
  }
}

export async function deleteDirectory(path: string): Promise<void> {
  markBusy(path);
  fs$.error.set(null);
  try {
    await fsService.deleteDir(path);
    // Invalidate any cached tree rooted at or under this path.
    const trees = fs$.treesByPath.peek();
    for (const key of Object.keys(trees)) {
      if (key === path || key.startsWith(`${path}/`)) {
        fs$.treesByPath[key].delete();
      }
    }
  } catch (e) {
    fs$.error.set(e instanceof Error ? e.message : "Failed to delete directory");
    throw e;
  } finally {
    clearBusy(path);
  }
}

export function clearError(): void {
  fs$.error.set(null);
}

export function clearFsState(): void {
  batch(() => {
    fs$.browse.set(null);
    fs$.browsing.set(false);
    fs$.treesByPath.set({} as Record<string, DirectoryTreeResult>);
    fs$.fileContentsByPath.set({} as Record<string, ReadFileResult>);
    fs$.busyPaths.set({} as Record<string, boolean>);
    fs$.error.set(null);
  });
}

/** Render a tree entry as an indented text line (for mobile tree previews). */
function formatTreeEntry(entry: FsTreeEntry, depth = 0): string {
  const indent = "  ".repeat(depth);
  const suffix = entry.isDir ? "/" : "";
  return `${indent}${entry.name}${suffix}`;
}
