/**
 * Desktop-specific filesystem response types.
 *
 * These mirror the `data` payload returned by the Console server's FS
 * routes. `FsTreeEntry` itself is shared via `@console/types`.
 */

import type { FsTreeEntry } from "@console/types";

export interface BrowseResult {
  path: string;
  parentPath: string | null;
  entries: FsTreeEntry[];
}

export interface PickFolderResult {
  path: string;
}

export interface DirectoryTreeResult {
  path: string;
  treeFormatted: string;
}

export interface ReadFileResult {
  path: string;
  content: string;
}

export interface WriteFileResult {
  path: string;
  message: string;
}

export interface DeleteFileResult {
  path: string;
  deleted: boolean;
}

export interface CreateDirectoryResult {
  path: string;
  created: boolean;
}

export interface DeleteDirectoryResult {
  path: string;
  deleted: boolean;
}
