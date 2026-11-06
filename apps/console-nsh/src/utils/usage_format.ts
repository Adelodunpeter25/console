// Usage value formatting, ported from console-ui's `common/usage_panel.rs`
// and console-core's `types/usage.rs`. The usage panel and the settings
// page's usage section both render these wire units.

import type { ContextSnapshot } from "../common/usage_meter";

/// console-core's `ContextSnapshot::used_vs_window` ("23.1k/1.0M").
export function usedVsWindow(snapshot: ContextSnapshot): string {
  return `${formatTokens(snapshot.usedTokens)}/${formatTokens(snapshot.contextWindow)}`;
}

function formatTokens(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`;
  return `${value}`;
}

/// console-core's `format_amount`: compact human units for each wire unit.
export function formatAmount(value: number, unit: string): string {
  switch (unit) {
    case "minutes": {
      const hours = value / 60;
      return hours >= 1 ? `${hours.toFixed(1)}h` : `${Math.round(value)}m`;
    }
    case "tokens":
      if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
      if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
      return `${Math.round(value)}`;
    case "percent":
      return `${Math.round(value)}%`;
    case "requests":
      return `${Math.round(value)}`;
    case "usd":
      return `$${value.toFixed(2)}`;
    default:
      return `${Math.round(value)}`;
  }
}

/// console-ui's `format_usage_value`: "12.5K/200.0K" where both ends exist,
/// "1.2h used" with only a used value, a percent otherwise.
export function formatUsageValue(limit: UsageLimitLike, percent: number): string {
  const amount = limit.amount;
  if (!amount) return `${Math.round(percent)}%`;
  if (amount.used != null && amount.limit != null && amount.unit !== "percent") {
    return `${formatAmount(amount.used, amount.unit ?? "")}/${formatAmount(amount.limit, amount.unit ?? "")}`;
  }
  if (amount.used != null && amount.unit !== "percent") {
    return `${formatAmount(amount.used, amount.unit ?? "")} used`;
  }
  return `${Math.round(percent)}%`;
}

interface UsageLimitLike {
  label?: string;
  amount?: {
    used?: number;
    limit?: number;
    unit?: string;
  } | null;
}
