// Formatting helpers for model presentation, ported from console-shared's
// `src/format.rs`.

/// "128000" -> "128k", "1000000" -> "1M".
export function formatContextWindow(contextWindow: number): string {
  return contextWindow >= 1_000_000
    ? `${Math.floor(contextWindow / 1_000_000)}M`
    : `${Math.floor(contextWindow / 1_000)}k`;
}

/// "claude-sonnet-4_5" -> "Claude Sonnet 4 5": split on `-`/`_`, capitalise
/// the first letter of each part.
export function formatModelName(modelId: string): string {
  return modelId
    .split(/[-_]/)
    .map((part) => (part ? part[0].toUpperCase() + part.slice(1) : ""))
    .join(" ");
}
