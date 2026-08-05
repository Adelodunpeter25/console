export function formatRelativeTime(dateInput?: string | number, compact = false): string {
  if (!dateInput) return "";
  const date = new Date(dateInput);
  const now = new Date();
  const diffSec = Math.floor((now.getTime() - date.getTime()) / 1000);
  if (isNaN(diffSec) || diffSec < 0) return compact ? "now" : "just now";
  if (diffSec < 60) return compact ? "now" : "just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return compact ? `${diffMin}m` : `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return compact ? `${diffHr}hr` : `${diffHr}h ago`;
  const diffDays = Math.floor(diffHr / 24);
  return compact ? `${diffDays}d` : `${diffDays}d ago`;
}

/** Calendar-day bucket for a date relative to today (local time). */
export function dayBucket(dateInput: string | number): number {
  const date = new Date(dateInput);
  const now = new Date();
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const startOfDay = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  return Math.floor((startOfToday.getTime() - startOfDay.getTime()) / 86_400_000);
}

const DAYS_MS = 86_400_000;

/** Human label for a calendar-day bucket, per the sidebar grouping spec. */
export function formatDayGroup(bucket: number): string {
  if (bucket <= 0) return "Today";
  if (bucket === 1) return "Yesterday";
  // The third distinct day shows its date (e.g. "Aug 2"), then rolls up.
  if (bucket === 2)
    return new Date(Date.now() - bucket * DAYS_MS).toLocaleDateString(undefined, {
      month: "short",
      day: "numeric",
    });
  if (bucket < 7) return "3 days ago";
  if (bucket < 30) return "1 week ago";
  if (bucket < 365) return "1 month ago";
  return "1 year ago";
}
