export const HOUR_MS = 60 * 60 * 1000;
export const DAY_MS = 24 * HOUR_MS;
export const WEEK_MS = 7 * DAY_MS;

function toNumber(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const n = Number(value);
    if (Number.isFinite(n)) return n;
  }
  return undefined;
}

export function parsePositiveTimestamp(value: unknown): number | undefined {
  const parsed = toNumber(value);
  if (parsed === undefined || parsed <= 0) return undefined;
  return parsed < 1_000_000_000_000 ? parsed * 1000 : parsed;
}

export function parseIsoTimestamp(value: unknown): number | undefined {
  if (typeof value !== "string" || !value) return undefined;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export function usageStatus(usedFraction: number | undefined): import("@console/types").UsageStatus {
  if (usedFraction === undefined) return "unknown";
  if (usedFraction >= 0.9) return "exhausted";
  if (usedFraction >= 0.5) return "warning";
  return "ok";
}
