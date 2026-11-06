// Tests for the pure visual-spec core, beside `visual/core.ts`: axis math,
// bucketing, tick formatting, the table's numeric heuristic, and the chart's
// one-line summary. The card drawing is a thin view pass over these.
import { expect, test } from "nsh/test";
import {
  describeChart,
  formatCompact,
  looksNumeric,
  minMaxBuckets,
  niceStep,
  valueAxis,
} from "./core";
import { parseVisual } from "./types";
import { metricTint } from "./metric";

test("visual valueAxis includes zero for bars and areas, pads lines", () => {
  const bars = valueAxis(20, 90, true, 5);
  expect(bars.min).toBe(0);
  expect(bars.max).toBeGreaterThanOrEqual(90);
  const line = valueAxis(20, 90, false, 5);
  expect(line.min).toBeLessThan(20);
  expect(line.max).toBeGreaterThan(90);
});

test("visual a flat series is expanded so it sits mid-plot", () => {
  const axis = valueAxis(5, 5, false, 5);
  expect(axis.min).toBeLessThan(5);
  expect(axis.max).toBeGreaterThan(5);
  expect(axis.ticks.length).toBeGreaterThan(0);
});

test("visual niceStep picks 1, 2 or 5 times a power of ten", () => {
  expect(niceStep(9, 5)).toBe(2);
  expect(niceStep(3, 5)).toBe(1);
  expect(niceStep(30, 5)).toBe(10);
});

test("visual formatCompact reads 1.2k, 3.4M, 2B and keeps small decimals", () => {
  expect(formatCompact(1200)).toBe("1.2k");
  expect(formatCompact(3_400_000)).toBe("3.4M");
  expect(formatCompact(2_000_000_000)).toBe("2B");
  expect(formatCompact(1.5)).toBe("1.5");
  expect(formatCompact(-2300)).toBe("-2.3k");
});

test("visual minMaxBuckets keeps a min and a max per bucket, passes small sets through", () => {
  const points = Array.from({ length: 100 }, (_, i) => ({ x: i, y: i % 7 }));
  const out = minMaxBuckets(points, 10);
  expect(out.length).toBe(20);
  expect(minMaxBuckets([{ x: 0, y: 1 }], 10).length).toBe(1);
});

test("visual describeChart summarizes kind, series, points, range and last value", () => {
  const chart = {
    kind: "line",
    title: "p50",
    series: [
      {
        name: "p50",
        points: [
          { x: 0, y: 140 },
          { x: 1, y: 128 },
        ],
      },
    ],
  };
  expect(describeChart(chart)).toBe(
    "Line chart, p50. 1 series, 2 points. Range 128 to 140. Last 128.",
  );
});

test("visual looksNumeric right-aligns numeric-looking cells only", () => {
  expect(looksNumeric("12,400")).toBe(true);
  expect(looksNumeric("$99.9")).toBe(true);
  expect(looksNumeric("-12%")).toBe(true);
  expect(looksNumeric("us-east")).toBe(false);
  expect(looksNumeric("")).toBe(false);
  expect(looksNumeric("---")).toBe(false);
});

test("visual parseVisual accepts v1 specs and rejects newer or malformed ones", () => {
  const ok = parseVisual({ version: 1, spec: { version: 1, blocks: [] } });
  expect(ok.tag).toBe("ok");
  expect(parseVisual({ version: 2, spec: { version: 2, blocks: [] } }).tag).toBe(
    "unsupported",
  );
  expect(parseVisual({ version: 1, spec: { version: 1, blocks: [{ no: 1 }] } }).tag).toBe(
    "invalid",
  );
  expect(parseVisual(null).tag).toBe("unsupported");
});

test("visual metric delta tint follows the trend and never relies on color alone", () => {
  expect(metricTint("up").arrow).toBe("▲ ");
  expect(metricTint("down").arrow).toBe("▼ ");
  expect(metricTint("flat").arrow).toBe("▬ ");
  expect(metricTint(undefined).arrow).toBe("");
});

test("visual the full fixture spec parses", () => {
  const full = {
    version: 1,
    spec: {
      version: 1,
      title: "Latency",
      subtitle: "p50 by day",
      blocks: [
        { type: "text", text: "Weekly rollout health." },
        { type: "metric", label: "p50", value: "128ms", delta: "-12%", trend: "down", caption: "vs last week" },
        { type: "metric", label: "uptime", value: "99.9%", delta: "+0.1%", trend: "up" },
        { type: "table", caption: "By region", columns: ["Region", "Requests", "Errors"], rows: [["us-east", "12,400", "18"], ["eu-west", "9,120", "7"]] },
        { type: "chart", kind: "line", title: "p50 over days", xLabel: "day", yLabel: "ms", series: [
          { name: "p50", points: [{ x: 0, y: 140, label: "Mon" }, { x: 1, y: 132, label: "Tue" }] },
          { name: "p95", points: [{ x: 0, y: 210, label: "Mon" }, { x: 1, y: 240, label: "Tue" }] },
        ] },
      ],
    },
  };
  const parsed = parseVisual(full);
  expect(parsed.tag).toBe("ok");
});
