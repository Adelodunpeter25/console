/**
 * Format an unknown value (from `ToolCall.arguments` / `ToolResult.content`)
 * as a displayable string. Strings pass through; everything else is
 * JSON-stringified for the UI.
 */
export function formatUnknown(value: unknown): string {
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2);
}
