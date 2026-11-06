// Time formatting for the sidebar, ported from console-ui's
// `utils/time.rs::format_time_ago`.

/// "just now", "5m", "3h", "2d", or "Nov 06" beyond a week.
/// `timestamp` is unix seconds or milliseconds (both arrive from the server).
export function formatTimeAgo(
  timestamp: number,
  now: number = Date.now() / 1000,
): string {
  const seconds0 = timestamp > 10_000_000_000 ? timestamp / 1000 : timestamp;
  const seconds = Math.max(0, Math.floor(now - seconds0));
  if (seconds <= 59) return "just now";
  if (seconds <= 3_599) return `${Math.floor(seconds / 60)}m`;
  if (seconds <= 86_399) return `${Math.floor(seconds / 3_600)}h`;
  if (seconds <= 604_799) return `${Math.floor(seconds / 86_400)}d`;
  const date = new Date(seconds0 * 1000);
  if (!isNaN(date.getTime())) {
    return date.toLocaleDateString(undefined, {
      month: "short",
      day: "2-digit",
    });
  }
  return `${Math.floor(seconds / 86_400)}d`;
}

/// Compact live-elapsed label for the transcript's working indicator:
/// "37s", "1m 5s", "1h 2m" — prose-free so it can tick every second beside
/// the pulsing dots (console-ui's `format_working_elapsed`).
export function formatWorkingElapsed(seconds: number): string {
  if (seconds <= 59) return `${seconds}s`;
  if (seconds <= 3_599) {
    const minutes = Math.floor(seconds / 60);
    const rest = seconds % 60;
    return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`;
  }
  const hours = Math.floor(seconds / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  return minutes === 0 ? `${hours}h` : `${hours}h ${minutes}m`;
}
