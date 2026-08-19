/**
 * Shared formatting utilities.
 */

/**
 * Extracts the base directory/folder name from a filesystem path.
 * Handles trailing slashes cleanly.
 * e.g. "/Users/user/Documents/Projects/console/" -> "console"
 */
export function folderName(path?: string | null): string {
  if (!path) return "";
  const trimmed = path.replace(/\/+$/, "");
  const lastIndex = trimmed.lastIndexOf("/");
  return lastIndex !== -1 ? trimmed.slice(lastIndex + 1) : trimmed;
}

/**
 * Formats model identifiers into clean human-readable names.
 * e.g. "claude-opus-4-6-thinking" -> "Claude Opus 4 6 Thinking"
 */
export function formatModelName(modelId?: string | null): string {
  if (!modelId) return "";
  return modelId
    .split(/[-_]/g)
    .map((part) => (part.length > 0 ? part[0]!.toUpperCase() + part.slice(1) : part))
    .join(" ");
}
