// Tests for the usage panel, beside `common/usage_panel.tsx`. Pure helpers
// are asserted directly; the panel itself is exercised through the meter's
// popover in `usage_panel.fixture.tsx`.
import { expect, open, test } from "nsh/test";
import {
  contextStatusColor,
  limitStatusColor,
  meterFraction,
} from "./usage_panel";
import {
  formatAmount,
  formatUsageValue,
  usedVsWindow,
} from "../utils/usage_format";

test("usedVsWindow renders tokens compactly", () => {
  expect(
    usedVsWindow({
      usedTokens: 23_100,
      contextWindow: 1_000_000,
      percentUsed: 2.3,
      thresholdRatio: 0.9,
    }),
  ).toBe("23.1k/1.0M");
});

test("formatAmount renders each wire unit", () => {
  expect(formatAmount(90, "minutes")).toBe("1.5h");
  expect(formatAmount(30, "minutes")).toBe("30m");
  expect(formatAmount(2_400_000, "tokens")).toBe("2.4M");
  expect(formatAmount(12_500, "tokens")).toBe("12.5K");
  expect(formatAmount(66.4, "percent")).toBe("66%");
  expect(formatAmount(42, "requests")).toBe("42");
  expect(formatAmount(1.5, "usd")).toBe("$1.50");
});

test("formatUsageValue shows both ends, used-only or a percent", () => {
  expect(
    formatUsageValue(
      { label: "w", amount: { used: 12_500, limit: 200_000, unit: "tokens" } },
      6,
    ),
  ).toBe("12.5K/200.0K");
  expect(
    formatUsageValue({ label: "w", amount: { used: 90, unit: "minutes" } }, 50),
  ).toBe("1.5h used");
  expect(
    formatUsageValue(
      { label: "w", amount: { unit: "percent", used: 44 } },
      44,
    ),
  ).toBe("44%");
  expect(formatUsageValue({ label: "w" }, 30)).toBe("30%");
});

test("the meter bar keeps a sliver above zero", () => {
  expect(meterFraction(0)).toBe(0);
  expect(meterFraction(1)).toBe(0.015);
  expect(meterFraction(50)).toBe(0.5);
  expect(meterFraction(150)).toBe(1);
});

test("context color tiers at the snapshot's threshold, limit color at 80/95", () => {
  expect(
    contextStatusColor({
      usedTokens: 1,
      contextWindow: 1,
      percentUsed: 55,
      thresholdRatio: 0.9,
    }),
  ).toBe("warning");
  expect(
    contextStatusColor({
      usedTokens: 1,
      contextWindow: 1,
      percentUsed: 91,
      thresholdRatio: 0.9,
    }),
  ).toBe("destructive");
  expect(
    limitStatusColor({ label: "w", amount: { used: 85, limit: 100 } }, 85),
  ).toBe("warning");
  expect(limitStatusColor({ label: "w", status: "exhausted" }, 10)).toBe(
    "destructive",
  );
  expect(
    limitStatusColor({ label: "w", amount: { used: 10, limit: 100 } }, 10),
  ).toBe("gauge");
});

test("the meter opens the panel: context, limits, reset labels", async () => {
  const app = await open("./usage_panel.fixture.tsx");
  expect(app.texts()).toContain("Shape: none");
  await app.press("label:Usage");
  // Nothing reported yet: the quota section shows the desktop's empty state.
  expect(app.texts()).toContain("No quota limits reported for this provider.");

  await app.press("label:Seed");
  expect(app.texts()).toContain("Shape: 1 limits");
  expect(app.texts()).toContain("Context");
  expect(app.texts()).toContain("23.1k/1.0M");
  expect(app.texts()).toContain("Usage Limits");
  expect(app.texts()).toContain("Five hour window");
  expect(app.texts()).toContain("resets in 3h 20m");
  expect(app.texts()).toContain("1.5h/5.0h");
});

test("the panel shows its loading state while the report is pending", async () => {
  const app = await open("./usage_panel.fixture.tsx");
  await app.press("label:Start loading");
  await app.press("label:Usage");
  expect(app.texts()).toContain("Loading usage data…");
});
