// Tests for the usage meter's pure core, beside `common/usage_meter.tsx`:
// the percent resolution chain and the fill-color tiers. The gauge drawing
// itself is a thin canvas pass over `usagePercent`.
import { expect, test } from "nsh/test";
import {
  gaugeFillColor,
  resolveUsedPercent,
  usagePercent,
  type UsageLimit,
} from "./usage_meter";

test("resolveUsedPercent prefers the explicit used fraction", () => {
  const limit: UsageLimit = {
    label: "w",
    amount: { usedFraction: 0.42, used: 1, limit: 1 },
  };
  expect(resolveUsedPercent(limit)).toBe(42);
});

test("resolveUsedPercent falls back to used over limit", () => {
  const limit: UsageLimit = { label: "w", amount: { used: 30, limit: 200 } };
  expect(resolveUsedPercent(limit)).toBe(15);
});

test("resolveUsedPercent reads a percent unit and inverts remaining", () => {
  expect(resolveUsedPercent({ label: "w", amount: { unit: "percent", used: 66.5 } })).toBe(
    66.5,
  );
  expect(resolveUsedPercent({ label: "w", amount: { remainingFraction: 0.25 } })).toBe(75);
});

test("resolveUsedPercent returns zero without a usable amount", () => {
  expect(resolveUsedPercent({ label: "w" })).toBe(0);
  expect(resolveUsedPercent({ label: "w", amount: null })).toBe(0);
  expect(resolveUsedPercent({ label: "w", amount: {} })).toBe(0);
});

test("usagePercent prefers the context snapshot and falls back to the first limit", () => {
  const report = { limits: [{ label: "w", amount: { used: 1, limit: 2 } }] };
  expect(
    usagePercent(report, {
      usedTokens: 1,
      contextWindow: 1,
      percentUsed: 99.9,
      thresholdRatio: 0.9,
    }),
  ).toBe(99.9);
  expect(usagePercent(report, null)).toBe(50);
  expect(usagePercent(null, null)).toBe(null);
});

test("the fill tiers with occupancy: gauge blue, warning at 80, danger at 95", () => {
  expect(gaugeFillColor(50)).toBe("gauge");
  expect(gaugeFillColor(80)).toBe("warning");
  expect(gaugeFillColor(95)).toBe("destructive");
  expect(gaugeFillColor(null)).toBe("gauge");
});
