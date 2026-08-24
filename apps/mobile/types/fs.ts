import type { FsTreeEntry } from "@console/types";

/** Browse result returned by GET /api/fs/browse. */
export interface BrowseResult {
  currentPath: string;
  parentPath: string | null;
  entries: FsTreeEntry[];
}

/** Directory tree returned by GET /api/fs/tree. */
export interface DirectoryTreeResult {
  path: string;
  treeFormatted: string;
}

/** File contents returned by GET /api/fs/file. */
export interface ReadFileResult {
  path: string;
  content: string;
}
