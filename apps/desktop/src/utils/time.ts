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
