// Canvas painter for the chart block, ported from console-ui's
// `chat/visual/paint.rs`: grid, axis labels, and the line/area/bar/scatter
// series. Axis math and point bucketing come from `./core`. GPUI's path
// builder maps onto nsh's Canvas 2D context; nsh's Canvas draw callback
// plays the role of the desktop's `canvas(...)` view.

import { token } from "nsh";
import type { CanvasRenderingContext2D, CanvasSize } from "nsh";
import { formatCompact, minMaxBuckets, valueAxis } from "./core";
import type { VisualChart } from "./types";

/// The desktop's series palette (`theme.chart`), carried over as constants:
/// nsh's chart tokens (`chart-1`..`chart-5`) are unset in `theme.json`, so
/// the card keeps the desktop's hues rather than graying out. Canvas draws
/// and legend dots share it.
export const CHART_COLORS = [
  "#E2795B", "#3B82F6", "#62C987", "#E0B36A",
  "#B583E8", "#4FC3C7", "#E86FA6", "#9AA64B",
] as const;

export function seriesColor(index: number): string {
  return CHART_COLORS[index % CHART_COLORS.length];
}

function tickColor(): string {
  // The desktop paints tick labels in `text_tertiary`; `ring` is nsh's
  // closest faint text token (`muted-foreground` reads as secondary).
  return token("ring");
}

function gridColor(): string {
  return token("border");
}

export function drawChart(
  cx: CanvasRenderingContext2D,
  size: CanvasSize,
  chart: VisualChart,
) {
  const all = chart.series.flatMap((s) => s.points);
  if (all.length === 0) return;
  const scatter = chart.kind === "scatter";
  const bars = chart.kind === "bar";
  const includeZero = bars || chart.kind === "area";
  const yLo = Math.min(...all.map((p) => p.y));
  const yHi = Math.max(...all.map((p) => p.y));
  const yAxis = valueAxis(yLo, yHi, includeZero, 5);
  const xMin = Math.min(...all.map((p) => p.x));
  const xMax = Math.max(...all.map((p) => p.x));
  const xAxis = scatter ? valueAxis(xMin, xMax, false, 5) : null;

  const w = size.width;
  const h = size.height;

  // Measure y labels to size the left gutter.
  cx.font = "10px monospace";
  const yLabels = yAxis.ticks.map((t) => formatCompact(t));
  const labelW = Math.max(0, ...yLabels.map((l) => cx.measureText(l).width));
  const [left, right, top, bottom] = [labelW + 10, 6, 6, 18];
  const pw = w - left - right;
  const ph = h - top - bottom;
  if (pw <= 0 || ph <= 0) return;
  const py = (v: number) => top + ph - ((v - yAxis.min) / (yAxis.max - yAxis.min)) * ph;
  let slots = 0;
  if (bars) {
    const xs = [...new Set(all.map((p) => Math.trunc(p.x * 1000)))];
    slots = Math.max(1, xs.length);
  }
  const pxOf = (x: number): number => {
    if (xAxis) return left + ((x - xAxis.min) / (xAxis.max - xAxis.min)) * pw;
    if (xMax === xMin) return left + pw / 2;
    const inset = bars ? pw / slots / 2 : 0;
    return left + inset + ((x - xMin) / (xMax - xMin)) * (pw - inset * 2);
  };
  const strokePath = (pts: [number, number][], width: number, color: string) => {
    if (pts.length === 0) return;
    cx.strokeStyle = color;
    cx.lineWidth = width;
    cx.beginPath();
    cx.moveTo(pts[0][0], pts[0][1]);
    for (const [x, y] of pts.slice(1)) cx.lineTo(x, y);
    cx.stroke();
  };

  // Grid + y labels.
  yAxis.ticks.forEach((t, i) => {
    const y = py(t);
    strokePath([[left, y], [left + pw, y]], 1, gridColor());
    const text = yLabels[i];
    cx.fillStyle = tickColor();
    cx.textAlign = "left";
    cx.fillText(text, left - 6 - cx.measureText(text).width, y - 6);
  });

  // X tick labels: numeric ticks for scatter, point labels (thinned) otherwise.
  let ticks: [number, string][];
  if (xAxis) {
    ticks = xAxis.ticks.map((t) => [pxOf(t), formatCompact(t)] as [number, string]);
  } else {
    const reference = chart.series.find((s) => s.points.length > 0)?.points ?? [];
    const step = Math.max(1, Math.floor(reference.length / 6));
    ticks = reference
      .filter((_, i) => i % step === 0)
      .map((p) => [pxOf(p.x), p.label ?? formatCompact(p.x)] as [number, string]);
  }
  let lastEnd = -Infinity;
  for (const [x, text] of ticks) {
    const tw = cx.measureText(text).width;
    const start = Math.min(Math.max(x - tw / 2, 0), w - tw);
    if (start > lastEnd + 4) {
      cx.fillStyle = tickColor();
      cx.textAlign = "left";
      cx.fillText(text, start, top + ph + 3);
      lastEnd = start + tw;
    }
  }

  const zeroY = py(Math.min(Math.max(0, yAxis.min), yAxis.max));
  const buckets = Math.max(1, Math.floor(pw));
  chart.series.forEach((s, si) => {
    const color = seriesColor(si);
    const pts =
      scatter || bars ? s.points : minMaxBuckets(s.points, buckets);
    if (chart.kind === "scatter") {
      cx.fillStyle = color;
      for (const p of pts) {
        const [px_, py_] = [pxOf(p.x), py(p.y)];
        const r = 3;
        cx.beginPath();
        cx.arc(px_, py_, r, 0, Math.PI * 2);
        cx.fill();
      }
    } else if (chart.kind === "bar") {
      const per = pw / slots;
      const bw = Math.min(24, (per * 0.7) / chart.series.length);
      cx.fillStyle = color;
      for (const p of pts) {
        const px_ = pxOf(p.x) + (si - (chart.series.length - 1) / 2) * bw;
        const y = py(p.y);
        const [topY, height] = [Math.min(y, zeroY), Math.max(1, Math.abs(zeroY - y))];
        cx.fillRect(px_ - bw / 2, topY, bw, height);
      }
    } else {
      const xy = pts.map((p) => [pxOf(p.x), py(p.y)] as [number, number]);
      if (xy.length === 1) {
        cx.fillStyle = color;
        cx.beginPath();
        cx.arc(xy[0][0], xy[0][1], 3, 0, Math.PI * 2);
        cx.fill();
      } else if (xy.length > 1) {
        if (chart.kind === "area") {
          cx.globalAlpha = 0.12;
          cx.fillStyle = color;
          cx.beginPath();
          cx.moveTo(xy[0][0], zeroY);
          for (const [x, y] of xy) cx.lineTo(x, y);
          cx.lineTo(xy[xy.length - 1][0], zeroY);
          cx.closePath();
          cx.fill();
          cx.globalAlpha = 1;
        }
        strokePath(xy, 1.5, color);
      }
    }
  });
}
