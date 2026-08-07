import type { FsTreeEntry } from "@console/types";

/**
 * Normalizes absolute or mixed entry paths into clean relative paths
 * starting from the given project root directory.
 *
 * Example:
 *   projectRoot: "/Users/dev/project"
 *   entry.path:  "/Users/dev/project/src/index.ts"
 *   Returns:     "src/index.ts"
 */
export function extractRelativePaths(entries: FsTreeEntry[], projectRoot?: string): string[] {
  const paths: string[] = [];
  const root = projectRoot ? projectRoot.replace(/\/$/, "") : "";

  function walk(items: FsTreeEntry[]) {
    for (const item of items) {
      let rel = item.path;
      if (root && rel.startsWith(root)) {
        rel = rel.slice(root.length).replace(/^\//, "");
      } else {
        rel = rel.replace(/^\//, "");
      }

      if (rel) {
        // If it's a directory, append a trailing slash for @pierre/trees path parsing
        paths.push(item.isDir ? `${rel}/` : rel);
      }

      if (item.children && item.children.length > 0) {
        walk(item.children);
      }
    }
  }

  walk(entries);
  return paths;
}
