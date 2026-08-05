import { create } from "zustand";
import { tauriApi } from "../lib/tauri-api";
import type { BrowseResult, DirectoryTreeResult, PickFolderResult, ReadFileResult } from "../types";

interface FsState {
  /** Current browse location and its entries (file-picker navigation). */
  browse: BrowseResult | null;
  browsing: boolean;
  /** Last folder picked via the native dialog. */
  pickedFolder: PickFolderResult | null;
  /** Cached directory trees keyed by path. */
  treesByPath: Record<string, DirectoryTreeResult>;
  /** Cached file contents keyed by path (line-range-agnostic). */
  fileContentsByPath: Record<string, ReadFileResult>;
  /** Set of paths with an in-flight operation. */
  busyPaths: Record<string, boolean>;
  error: string | null;

  browseDirectory: (path?: string) => Promise<BrowseResult>;
  pickFolder: () => Promise<PickFolderResult>;
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
  pickedFolder: null,
  treesByPath: {},
  fileContentsByPath: {},
  busyPaths: {},
  error: null,

  browseDirectory: async (path?: string) => {
    set({ browsing: true, error: null });
    try {
      const result = await tauriApi.browseDirectory(path);
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

  pickFolder: async () => {
    set({ error: null });
    const result = await tauriApi.pickFolder();
    set({ pickedFolder: result });
    return result;
  },

  getDirectoryTree: async (path?: string, depth?: number) => {
    const key = path ?? "";
    const cached = get().treesByPath[key];
    if (cached) return cached;

    set({ error: null });
    const result = await tauriApi.getDirectoryTree(path, depth);
    set((s) => ({ treesByPath: { ...s.treesByPath, [key]: result } }));
    return result;
  },

  readFile: async (path, startLine, endLine) => {
    const cached = get().fileContentsByPath[path];
    if (cached && !startLine && !endLine) return cached;

    set({ error: null });
    const result = await tauriApi.readFile(path, startLine, endLine);
    if (!startLine && !endLine) {
      set((s) => ({ fileContentsByPath: { ...s.fileContentsByPath, [path]: result } }));
    }
    return result;
  },

  writeFile: async (path, content) => {
    markBusy(set, path);
    set({ error: null });
    try {
      await tauriApi.writeFile(path, content);
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
      await tauriApi.deleteFile(path);
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
      await tauriApi.createDirectory(path);
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
      await tauriApi.deleteDirectory(path);
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
