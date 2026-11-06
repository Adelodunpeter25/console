// Pure geometry and text for the v1 visual spec, ported from console-core's
// `types/visual.rs` (axis math, point bucketing, table heuristics, the
// chart's one-line summary). No view code here so everything stays
// unit-testable; `visual.tsx` and `paint.tsx` do the showing.

import type { VisualChart, VisualPoint } from "./types";

function trimDecimals(value: number, decimals: number): string {
  const s = value.toFixed(decimals);
  return s.includes(".") ? s.replace(/0+$/, "").replace(/\.$/, "") : s;
}

/// Compact tick text: 1.2k, 3.4M; small values keep up to two decimals.
export function formatCompact(v: number): string {
  const a = Math.abs(v);
  if (a >= 1e9) return `${trimDecimals(v / 1e9, 1)}B`;
  if (a >= 1e6) return `${trimDecimals(v / 1e6, 1)}M`;
  if (a >= 1e3) return `${trimDecimals(v / 1e3, 1)}k`;
  return trimDecimals(v, 2);
}

/// Value axis: `min`/`max` plus the tick values between them.
export interface Axis {
  min: number;
  max: number;
  ticks: number[];
}

export function niceStep(range: number, targetTicks: number): number {
  const raw = range / Math.max(1, targetTicks);
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const norm = raw / mag;
  const nice = norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 5 ? 5 : 10;
  return nice * mag;
}

/// Bars and areas include zero; lines and scatter use the data range with
/// padding. A flat series is expanded so it sits mid-plot.
export function valueAxis(lo: number, hi: number, includeZero: boolean, targetTicks: number): Axis {
  let [a, b] = [lo, hi];
  if (includeZero) {
    a = Math.min(a, 0);
    b = Math.max(b, 0);
  }
  if (a === b) {
    const pad = a === 0 ? 1 : Math.abs(a) * 0.5;
    a -= pad;
    b += pad;
  } else if (!includeZero) {
    const pad = (b - a) * 0.05;
    a -= pad;
    b += pad;
  }
  const step = niceStep(b - a, targetTicks);
  const niceMin = Math.floor(a / step) * step;
  const niceMax = Math.ceil(b / step) * step;
  const ticks: number[] = [];
  let t = niceMin;
  while (t <= niceMax + step * 1e-9 && ticks.length < 40) {
    ticks.push(Math.abs(t) < step * 1e-9 ? 0 : t);
    t += step;
  }
  return { min: niceMin, max: niceMax, ticks };
}

/// Rendering safety net: at most a min and a max per horizontal bucket, so a
/// dense line never draws more segments than pixels. Under the cap the
/// points pass through untouched.
export function minMaxBuckets(points: VisualPoint[], buckets: number): VisualPoint[] {
  if (buckets < 1 || points.length <= buckets * 2) return [...points];
  const per = points.length / buckets;
  const out: VisualPoint[] = [];
  for (let i = 0; i < buckets; i++) {
    const from = Math.floor(i * per);
    const to = Math.max(from + 1, Math.min(Math.floor((i + 1) * per), points.length));
    let [lo, hi] = [from, from];
    for (let j = from; j < to; j++) {
      if (points[j].y < points[lo].y) lo = j;
      if (points[j].y > points[hi].y) hi = j;
    }
    const [first, second] = lo <= hi ? [lo, hi] : [hi, lo];
    out.push(points[first]);
    if (second !== first) out.push(points[second]);
  }
  return out;
}

/// One-line summary shown under a chart (GPUI had no accessibility tree, so
/// this is also the fallback for anyone who cannot read the plot).
export function describeChart(chart: VisualChart): string {
  const all = chart.series.flatMap((s) => s.points);
  const kind = chart.kind ? chart.kind[0].toUpperCase() + chart.kind.slice(1) : "";
  const title = chart.title ? `, ${chart.title}` : "";
  const perSeries = Math.max(0, ...chart.series.map((s) => s.points.length));
  const seriesWord = chart.series.length === 1 ? "series" : "series";
  const pointWord = perSeries === 1 ? "point" : "points";
  let out = `${kind} chart${title}. ${chart.series.length} ${seriesWord}, ${perSeries} ${pointWord}.`;
  if (all.length > 0) {
    const lo = Math.min(...all.map((p) => p.y));
    const hi = Math.max(...all.map((p) => p.y));
    out += ` Range ${formatCompact(lo)} to ${formatCompact(hi)}.`;
    const last = chart.series.at(-1)?.points.at(-1);
    if (last) out += ` Last ${formatCompact(last.y)}.`;
  }
  return out;
}

/// Heuristic from the plan: numeric-looking cells right-align.
export function looksNumeric(cell: string): boolean {
  const t = cell
    .trim()
    .replace(/^[+\-$€£]+/, "")
    .replace(/[%kKMB]+$/, "");
  return t.length > 0 && /^[0-9.,]+$/.test(t) && /[0-9]/.test(t);
}
