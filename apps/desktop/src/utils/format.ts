/**
 * Format an unknown value (from `ToolCall.arguments` / `ToolResult.content`)
 * as a displayable string. Strings pass through; everything else is
 * JSON-stringified for the UI.
 */
export function formatUnknown(value: unknown): string {
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2);
}

/**
 * Last path segment of a working directory, e.g. /a/b/console -> Console.
 * The first letter is capitalised for display.
 */
export function basename(cwd?: string): string {
  if (!cwd) return "No folder";
  const parts = cwd.split(/[\\/]/).filter(Boolean);
  const last = parts[parts.length - 1] ?? cwd;
  return last.charAt(0).toUpperCase() + last.slice(1);
}
