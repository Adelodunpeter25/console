// The usage panel, ported from console-ui's `common/usage_panel.rs`: a
// dropdown showing detailed quota limits for the active provider — progress
// bars per limit, reset times and status colors, plus the session's context
// occupancy. The meter's popover shows it. The value formatting lives in
// `utils/usage_format.ts` (the settings page will reuse it); the UI-side
// helpers here (status tiers, the bar's sliver fraction) are tested directly.

import { Text, View, token } from "nsh";
import type {
  ContextSnapshot,
  UsageLimit,
  UsageReport,
} from "./usage_meter";
import { resolveUsedPercent } from "./usage_meter";
import {
  formatUsageValue,
  usedVsWindow,
} from "../utils/usage_format";

/// The meter bar's fill fraction: clamped, keeping a visible sliver above
/// zero (the desktop's 1.5% floor).
export function meterFraction(percent: number): number {
  const fraction = Math.min(1, Math.max(0, percent / 100));
  return fraction > 0 ? Math.max(0.015, fraction) : 0;
}

/// The context bar's color: danger past the session's threshold, warning at
/// 50%, the gauge blue otherwise.
export function contextStatusColor(
  snapshot: ContextSnapshot,
): "destructive" | "warning" | "gauge" {
  const percent = Math.min(100, Math.max(0, snapshot.percentUsed));
  const threshold = Math.min(100, Math.max(0, snapshot.thresholdRatio * 100));
  if (percent >= threshold) return "destructive";
  if (percent >= 50) return "warning";
  return "gauge";
}

/// A limit row's color: exhausted or 95%+ danger, warning status or 80%+
/// warning, the gauge blue otherwise.
export function limitStatusColor(
  limit: UsageLimit,
  percent: number,
): "destructive" | "warning" | "gauge" {
  if (percent >= 95 || limit.status === "exhausted") return "destructive";
  if (percent >= 80 || limit.status === "warning") return "warning";
  return "gauge";
}

/// The horizontal progress bar under each section: a 3px rounded track with
/// the fill colored by status.
function MeterBar(props: { percent: number; color: "destructive" | "warning" | "gauge" }) {
  const fraction = meterFraction(props.percent);
  return (
    <View class="h-[3px] w-full flex-none rounded-full bg-muted">
      <View
        class="h-full rounded-full"
        style={{ width: `${fraction * 100}%`, backgroundColor: token(props.color) }}
      />
    </View>
  );
}

/// The empty/loading states the quota section cycles through.
function QuotaNote(props: { children: string }) {
  return (
    <Text class="py-3 text-center text-[12px] text-muted-foreground">
      {props.children}
    </Text>
  );
}

function LimitRow(props: { limit: UsageLimit }) {
  const limit = props.limit;
  const percent = resolveUsedPercent(limit);
  const resetLabel = limit.window?.resetLabel;
  return (
    <View class="flex-col gap-[7px]">
      <View class="flex-row items-center justify-between gap-2">
        <View class="min-w-0 flex-1 overflow-hidden">
          <Text class="text-[12.5px] text-foreground">{limit.label}</Text>
        </View>
        {resetLabel ? (
          <Text class="flex-none text-[11.5px] text-muted-foreground">
            {resetLabel}
          </Text>
        ) : null}
        <Text class="flex-none text-[12px] text-muted-foreground">
          {formatUsageValue(limit, percent)}
        </Text>
      </View>
      <MeterBar percent={percent} color={limitStatusColor(limit, percent)} />
    </View>
  );
}

/// The panel card: the context section on top, the provider's quota limits
/// under a divider. The report/snapshot are thunks because the popover's
/// content mounts once and the values can arrive after it.
export function UsagePanel(props: {
  usageReport: () => UsageReport | null;
  contextSnapshot: () => ContextSnapshot | null;
  isLoading: () => boolean;
}) {
  // Everything stays behind thunks: the popover's content mounts once, so
  // values read at body level would freeze at first open.
  const report = () => props.usageReport();
  const snapshot = () => props.contextSnapshot();
  const percent = () => {
    const snap = snapshot();
    return snap ? Math.min(100, Math.max(0, snap.percentUsed)) : null;
  };

  return (
    <View class="w-[320px] flex-col gap-3 rounded-[10px] border border-border bg-popover p-3.5 shadow-lg">
      <View class="flex-row items-center justify-between">
        <Text class="text-[12.5px] font-semibold text-foreground">Context</Text>
        {snapshot() ? (
          <Text class="text-[12px] text-muted-foreground">
            {usedVsWindow(snapshot()!)}
          </Text>
        ) : null}
      </View>
      {snapshot() && percent() != null ? (
        <MeterBar percent={percent()!} color={contextStatusColor(snapshot()!)} />
      ) : (
        <Text class="text-[12px] text-muted-foreground">
          Context usage unavailable.
        </Text>
      )}

      <View class="flex-col gap-2.5 border-t border-border pt-2">
        <Text class="text-[12.5px] font-semibold text-foreground">
          Usage Limits
        </Text>
        {props.isLoading() && !report() ? (
          <QuotaNote>Loading usage data…</QuotaNote>
        ) : !report() ? (
          <QuotaNote>No quota limits reported for this provider.</QuotaNote>
        ) : report()!.limits.length === 0 ? (
          <QuotaNote>No active rate limit windows.</QuotaNote>
        ) : (
          <View class="flex-col gap-3">
            {report()!.limits.map((limit) => (
              <LimitRow limit={limit} />
            ))}
          </View>
        )}
      </View>
    </View>
  );
}
