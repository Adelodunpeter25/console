import { theme } from "@/styles/theme";
import type { UsageLimit } from "@console/types";

export function formatResetsAt(resetsAt?: number): string | null {
  if (!resetsAt) return null;
  const diff = resetsAt - Date.now();
  if (diff <= 0) return "resetting…";
  const hours = Math.floor(diff / (1000 * 60 * 60));
  const mins = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    return `resets in ${days}d ${hours % 24}h`;
  }
  if (hours > 0) return `resets in ${hours}h ${mins}m`;
  return `resets in ${mins}m`;
}

export function formatWindowLabel(limit: UsageLimit): string {
  const windowLabel = limit.window?.label ?? limit.window?.id ?? "Quota";
  const resets = formatResetsAt(limit.window?.resetsAt);
  return resets ? `${windowLabel} · ${resets}` : windowLabel;
}

export function statusColor(status?: string): string {
  switch (status) {
    case "exhausted":
      return "#f87171";
    case "warning":
      return "#fbbf24";
    case "ok":
      return "#34d399";
    default:
      return theme.colors.text.muted;
  }
}

export function getUsedPercent(limit: UsageLimit): number | null {
  if (limit.amount.usedFraction !== undefined) return Math.round(limit.amount.usedFraction * 100);
  if (limit.amount.used !== undefined) return Math.round(limit.amount.used);
  return null;
}

export function getBarPercent(limit: UsageLimit): number {
  if (limit.amount.usedFraction !== undefined) return Math.min(100, Math.max(0, limit.amount.usedFraction * 100));
  if (limit.amount.remainingFraction !== undefined) return Math.min(100, Math.max(0, (1 - limit.amount.remainingFraction) * 100));
  return 0;
}
