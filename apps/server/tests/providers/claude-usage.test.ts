/**
 * Claude usage provider tests.
 * Covers OAuth usage payload calculation (5h/7d, scoped weekly, extra spend)
 * and unified rate-limit header parsing. All tests are offline — fetch is mocked.
 */
import assert from "node:assert/strict";
import {
  claudeUsageProvider,
  parseClaudeRateLimitHeaders,
} from "@/providers/src/usage/claude.js";

console.log("Running Claude usage tests...");

function mockFetch(payload: unknown, status = 200) {
  return (async () =>
    new Response(JSON.stringify(payload), {
      status,
      headers: { "Content-Type": "application/json" },
    })) as unknown as typeof fetch;
}

const credential = {
  type: "oauth" as const,
  accessToken: "test-token",
  expiresAt: Date.now() + 60_000,
  email: "user@example.com",
};

// Legacy buckets + scoped weekly limit.
{
  const now = Date.now();
  const report = await claudeUsageProvider.fetchUsage(
    { provider: "claude", credential },
    {
      fetch: mockFetch({
        five_hour: { utilization: 16, resets_at: new Date(now + 5 * 3_600_000).toISOString() },
        seven_day: { utilization: 18, resets_at: new Date(now + 3 * 86_400_000).toISOString() },
        seven_day_opus: null,
        seven_day_sonnet: null,
        limits: [
          {
            kind: "weekly_scoped",
            percent: 28,
            resets_at: new Date(now + 4 * 86_400_000).toISOString(),
            scope: { model: { display_name: "Fable" } },
          },
        ],
      }),
    },
  );
  assert.ok(report, "expected report");
  assert.deepEqual(
    report.limits.map((l) => l.id),
    ["claude:5h", "claude:7d", "claude:7d:fable"],
  );
  const fable = report.limits.find((l) => l.id === "claude:7d:fable");
  assert.equal(fable?.scope.tier, "fable");
  assert.equal(fable?.scope.shared, undefined);
  assert.equal(fable?.amount.used, 28);
  assert.equal(fable?.amount.limit, 100);
  assert.equal(fable?.status, "ok");
  const fiveHour = report.limits.find((l) => l.id === "claude:5h");
  assert.equal(fiveHour?.scope.shared, true);
  assert.equal(fiveHour?.amount.used, 16);
}

// session/weekly_all fallback when legacy buckets are absent.
{
  const now = Date.now();
  const report = await claudeUsageProvider.fetchUsage(
    { provider: "claude", credential },
    {
      fetch: mockFetch({
        limits: [
          {
            kind: "session",
            percent: 10,
            resets_at: new Date(now + 3_600_000).toISOString(),
            scope: null,
          },
          {
            kind: "weekly_all",
            percent: 20,
            resets_at: new Date(now + 86_400_000).toISOString(),
            scope: null,
          },
        ],
      }),
    },
  );
  assert.ok(report, "expected fallback report");
  assert.deepEqual(
    report.limits.map((l) => l.id),
    ["claude:5h", "claude:7d"],
  );
}

// Capped extra spend calculation.
{
  const report = await claudeUsageProvider.fetchUsage(
    { provider: "claude", credential },
    {
      fetch: mockFetch({
        five_hour: { utilization: 5 },
        spend: {
          enabled: true,
          used: { amount_minor: 45_000, currency: "USD", exponent: 2 },
          limit: { amount_minor: 50_000, currency: "USD", exponent: 2 },
        },
      }),
    },
  );
  const extra = report?.limits.find((l) => l.id === "claude:extra");
  assert.deepEqual(extra?.amount, {
    used: 450,
    limit: 500,
    remaining: 50,
    usedFraction: 0.9,
    remainingFraction: 0.1,
    unit: "usd",
  });
  assert.equal(extra?.status, "warning");
}

// Over-cap spend is exhausted with zero remaining.
{
  const report = await claudeUsageProvider.fetchUsage(
    { provider: "claude", credential },
    {
      fetch: mockFetch({
        five_hour: { utilization: 5 },
        spend: {
          enabled: true,
          used: { amount_minor: 62_500, currency: "USD", exponent: 2 },
          limit: { amount_minor: 50_000, currency: "USD", exponent: 2 },
        },
      }),
    },
  );
  const extra = report?.limits.find((l) => l.id === "claude:extra");
  assert.equal(extra?.amount.usedFraction, 1.25);
  assert.equal(extra?.amount.remaining, 0);
  assert.equal(extra?.status, "exhausted");
}

// Unified rate-limit headers map to shared + Fable windows.
{
  const now = 1_780_400_000_000;
  const report = parseClaudeRateLimitHeaders(
    {
      "anthropic-ratelimit-unified-5h-utilization": "0.02",
      "anthropic-ratelimit-unified-5h-reset": "1780405800",
      "anthropic-ratelimit-unified-7d-utilization": "0.3",
      "anthropic-ratelimit-unified-7d-reset": "1780531200",
      "anthropic-ratelimit-unified-7d_oi-utilization": "0.55",
      "anthropic-ratelimit-unified-7d_oi-reset": "1780617600",
    },
    now,
  );
  assert.ok(report, "expected header report");
  assert.deepEqual(
    report.limits.map((l) => l.id),
    ["claude:5h", "claude:7d", "claude:7d:fable"],
  );
  assert.equal(report.limits[0]?.amount.used, 2);
  assert.equal(report.limits[0]?.window?.resetsAt, 1780405800 * 1000);
  const fable = report.limits.find((l) => l.id === "claude:7d:fable");
  assert.equal(fable?.scope.tier, "fable");
  assert.ok(Math.abs((fable?.amount.used ?? 0) - 55) < 1e-9);
}

// No headers → null.
{
  assert.equal(parseClaudeRateLimitHeaders({}), null);
}

// Non-OAuth credentials are rejected.
{
  const report = await claudeUsageProvider.fetchUsage(
    { provider: "claude", credential: { type: "api_key", apiKey: "x" } },
    { fetch: mockFetch({}) },
  );
  assert.equal(report, null);
}

console.log("Claude usage tests passed.");
