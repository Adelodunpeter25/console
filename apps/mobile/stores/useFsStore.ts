import { create } from "zustand";
import type { FsTreeEntry } from "@console/types";
import { fsService } from "@console/api";
import type { BrowseResult, DirectoryTreeResult, ReadFileResult } from "@/types";

interface FsState {
  /** Current browse location and its entries (file-picker navigation). */
  browse: BrowseResult | null;
  browsing: boolean;
  /** Cached directory trees keyed by path. */
  treesByPath: Record<string, DirectoryTreeResult>;
  /** Cached file contents keyed by path (line-range-agnostic). */
  fileContentsByPath: Record<string, ReadFileResult>;
  /** Set of paths with an in-flight operation. */
  busyPaths: Record<string, boolean>;
  error: string | null;

  browseDirectory: (path?: string) => Promise<BrowseResult>;
  getDirectoryTree: (path?: string, depth?: number) => Promise<DirectoryTreeResult>;
  readFile: (path: string, startLine?: number, endLine?: number) => Promise<ReadFileResult>;
  writeFile: (path: string, content: string) => Promise<void>;
  deleteFile: (path: string) => Promise<void>;
  createDirectory: (path: string) => Promise<void>;
  deleteDirectory: (path: string) => Promise<void>;
  clearError: () => void;
}

function markBusy(set: (partial: Partial<FsState>) => void, path: string): void {
  set({ busyPaths: { ...useFsStore.getState().busyPaths, [path]: true } });
}

function clearBusy(set: (partial: Partial<FsState>) => void, path: string): void {
  const next = { ...useFsStore.getState().busyPaths };
  delete next[path];
  set({ busyPaths: next });
}

export const useFsStore = create<FsState>((set, get) => ({
  browse: null,
  browsing: false,
  treesByPath: {},
  fileContentsByPath: {},
  busyPaths: {},
  error: null,

  browseDirectory: async (path?: string) => {
    set({ browsing: true, error: null });
    try {
      const result = await fsService.getFsBrowse(path);
      set({ browse: result, browsing: false });
      return result;
    } catch (e) {
      set({
        browsing: false,
        error: e instanceof Error ? e.message : "Failed to browse directory",
      });
      throw e;
    }
  },

  getDirectoryTree: async (path?: string, depth?: number) => {
    const key = path ?? "";
    const cached = get().treesByPath[key];
    if (cached) return cached;

    set({ error: null });
    const result = await fsService.getFsTree(path);
    // Convert the flat tree list into the desktop-shaped tree result.
    const treeResult: DirectoryTreeResult = {
      path: path ?? "",
      treeFormatted: result
        .map((entry: FsTreeEntry) => formatTreeEntry(entry))
        .join("\n"),
    };
    set((s) => ({ treesByPath: { ...s.treesByPath, [key]: treeResult } }));
    return treeResult;
  },

  readFile: async (path, startLine, endLine) => {
    const cached = get().fileContentsByPath[path];
    if (cached && !startLine && !endLine) return cached;

    set({ error: null });
    const result = await fsService.readFile(path);
    const readResult: ReadFileResult = {
      path: result.path,
      content: result.content,
    };
    if (!startLine && !endLine) {
      set((s) => ({ fileContentsByPath: { ...s.fileContentsByPath, [path]: readResult } }));
    }
    return readResult;
  },

  writeFile: async (path, content) => {
    markBusy(set, path);
    set({ error: null });
    try {
      await fsService.writeFile(path, content);
      // Invalidate cached content so the next read fetches the new state.
      set((s) => {
        const next = { ...s.fileContentsByPath };
        delete next[path];
        return { fileContentsByPath: next };
      });
    } catch (e) {
      set({ error: e instanceof Error ? e.message : "Failed to write file" });
      throw e;
    } finally {
      clearBusy(set, path);
    }
  },

  deleteFile: async (path) => {
    markBusy(set, path);
    set({ error: null });
    try {
      await fsService.deleteFile(path);
      set((s) => {
        const contents = { ...s.fileContentsByPath };
        delete contents[path];
        return { fileContentsByPath: contents };
      });
    } catch (e) {
      set({ error: e instanceof Error ? e.message : "Failed to delete file" });
      throw e;
    } finally {
      clearBusy(set, path);
    }
  },

  createDirectory: async (path) => {
    markBusy(set, path);
    set({ error: null });
    try {
      await fsService.createDir(path);
    } catch (e) {
      set({ error: e instanceof Error ? e.message : "Failed to create directory" });
      throw e;
    } finally {
      clearBusy(set, path);
    }
  },

  deleteDirectory: async (path) => {
    markBusy(set, path);
    set({ error: null });
    try {
      await fsService.deleteDir(path);
      // Invalidate any cached tree rooted at or under this path.
      set((s) => {
        const trees = { ...s.treesByPath };
        for (const key of Object.keys(trees)) {
          if (key === path || key.startsWith(`${path}/`)) {
            delete trees[key];
          }
        }
        return { treesByPath: trees };
      });
    } catch (e) {
      set({ error: e instanceof Error ? e.message : "Failed to delete directory" });
      throw e;
    } finally {
      clearBusy(set, path);
    }
  },

  clearError: () => set({ error: null }),
}));

/** Render a tree entry as an indented text line (for mobile tree previews). */
function formatTreeEntry(entry: FsTreeEntry, depth = 0): string {
  const indent = "  ".repeat(depth);
  const suffix = entry.isDir ? "/" : "";
  return `${indent}${entry.name}${suffix}`;
}
