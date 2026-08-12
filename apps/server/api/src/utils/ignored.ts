/**
 * Common directories and files that should be ignored by the file watcher
 * and file tree processing to avoid event loops and performance degradation.
 */
export const IGNORED_PATHS = [
  "node_modules",
  ".git",
  ".hg",
  ".svn",
  "dist",
  "build",
  ".next",
  ".turbo",
  ".vite",
  ".vite-temp",
  ".cache",
  "coverage",
  ".DS_Store",
  "thumbs.db",
  ".gemini",
  "target",
  "tmp",
];

/**
 * Check if a given relative or absolute path contains any ignored directory/file segment.
 */
export function isPathIgnored(filePath: string): boolean {
  if (!filePath) return false;
  const normalized = filePath.replace(/\\/g, "/");
  const segments = normalized.split("/");
  return segments.some((segment) =>
    IGNORED_PATHS.some((ignored) => segment.toLowerCase() === ignored.toLowerCase()),
  );
}
