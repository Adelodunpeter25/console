// The usage meter, ported from console-ui's `common/usage_meter.rs`: a small
// circular gauge in the workspace footer showing the active provider's
// rate-limit occupancy — context-window usage when known, the primary quota
// limit otherwise. Pressing it opens the detailed usage panel
// (`usage_panel.tsx`), as the desktop's does.

import { Canvas, View, token } from "nsh";
import type { CanvasRenderingContext2D } from "nsh";
import { Popover } from "nsh/base";
import { UsagePanel } from "./usage_panel";
import { Hint } from "./tooltip";

/// The shapes the meter reads, mirroring console-core's `ContextSnapshot`
/// (`percentUsed`) and `UsageReport`/`UsageLimit`/`Amount`. The Rust side
/// serializes them; these are the wire values.
export interface ContextSnapshot {
  usedTokens: number;
  contextWindow: number;
  percentUsed: number;
  thresholdRatio: number;
}

export interface UsageAmount {
  used?: number;
  limit?: number;
  usedFraction?: number;
  remainingFraction?: number;
  unit?: string;
}

export interface UsageLimit {
  label: string;
  status?: string;
  amount?: UsageAmount | null;
  window?: { resetLabel?: string } | null;
}

export interface UsageReport {
  limits: UsageLimit[];
}

/// console-core's `Amount::used_percent` chain: explicit used fraction, then
/// used/limit, then a percent unit, then the remaining fraction inverted.
export function resolveUsedPercent(limit: UsageLimit | undefined): number {
  const amount = limit?.amount;
  if (!amount) return 0;
  if (amount.usedFraction != null) {
    return clamp(amount.usedFraction * 100);
  }
  if (amount.used != null && amount.limit != null && amount.limit > 0) {
    return clamp((amount.used * 100) / amount.limit);
  }
  if (amount.unit === "percent" && amount.used != null) {
    return clamp(amount.used);
  }
  if (amount.remainingFraction != null) {
    return clamp((1 - amount.remainingFraction) * 100);
  }
  return 0;
}

/// The gauge's occupancy: the context snapshot's percent when known, the
/// first limit's otherwise; `null` when nothing is known (track ring only).
export function usagePercent(
  report: UsageReport | null | undefined,
  snapshot: ContextSnapshot | null | undefined,
): number | null {
  if (snapshot) return clamp(snapshot.percentUsed);
  return report ? resolveUsedPercent(report.limits[0]) : null;
}

function clamp(percent: number): number {
  return Math.min(100, Math.max(0, percent));
}

/// The fill color tiers with occupancy: danger at 95%, warning at 80%,
/// the gauge blue otherwise. The desktop's thresholds and palette.
export function gaugeFillColor(percent: number | null): "destructive" | "warning" | "gauge" {
  if (percent != null && percent >= 95) return "destructive";
  if (percent != null && percent >= 80) return "warning";
  return "gauge";
}

/// The circular gauge glyph: a 13px ring whose arc fills clockwise from 12
/// o'clock over a faint track ring; a nearly-empty context keeps a sliver.
function drawGauge(
  cx: CanvasRenderingContext2D,
  size: { width: number; height: number },
  percent: number | null,
  track: string,
  fill: string,
) {
  const stroke = 2.5;
  const center = size.width / 2;
  const radius = (Math.min(size.width, size.height) - stroke) / 2;

  cx.lineWidth = stroke;
  cx.strokeStyle = track;
  cx.beginPath();
  cx.arc(center, center, radius, 0, Math.PI * 2);
  cx.stroke();

  if (percent == null) return;

  // Keep a visible sliver for a nearly-empty context.
  const fraction = Math.max(0.05, Math.min(1, percent / 100));
  cx.strokeStyle = fill;
  cx.beginPath();
  const start = -Math.PI / 2;
  if (fraction >= 0.999) {
    cx.arc(center, center, radius, 0, Math.PI * 2);
  } else {
    cx.arc(center, center, radius, start, start + fraction * Math.PI * 2);
  }
  cx.stroke();
}

export function UsageMeter(props: {
  usageReport: UsageReport | null | (() => UsageReport | null);
  contextSnapshot: ContextSnapshot | null | (() => ContextSnapshot | null);
  isLoading?: boolean | (() => boolean);
}) {
  const report = () =>
    typeof props.usageReport === "function"
      ? props.usageReport()
      : props.usageReport;
  const snapshot = () =>
    typeof props.contextSnapshot === "function"
      ? props.contextSnapshot()
      : props.contextSnapshot;
  const isLoading = () =>
    typeof props.isLoading === "function" ? props.isLoading() : !!props.isLoading;

  const percent = () => usagePercent(report(), snapshot());

  return (
    <Popover
      placement="top"
      align="end"
      popupClass="rounded-xl"
      trigger={
        <View
          class="h-[22px] flex-row items-center justify-center rounded px-1.5 hover:bg-accent"
          accessibilityLabel="Usage"
          tooltip={<Hint>Usage</Hint>}
        >
          <Canvas
            class="h-[13px] w-[13px]"
            draw={(cx, size) =>
              drawGauge(
                cx,
                size,
                percent(),
                token("border"),
                token(gaugeFillColor(percent())),
              )
            }
          />
        </View>
      }
    >
      <UsagePanel
        usageReport={report}
        contextSnapshot={snapshot}
        isLoading={isLoading}
      />
    </Popover>
  );
}
