import type {
  UsageAmount,
  UsageFetchContext,
  UsageFetchParams,
  UsageLimit,
  UsageProvider,
  UsageReport,
  UsageWindow,
} from "@console/types";
import { CLAUDE_CODE_VERSION } from "@/providers/src/claude/constants.js";
import { HOUR_MS, WEEK_MS, parseIsoTimestamp } from "./shared.js";

const DEFAULT_ENDPOINT = "https://api.anthropic.com/api/oauth";

const CLAUDE_HEADERS = {
  accept: "application/json, text/plain, */*",
  "content-type": "application/json",
  "user-agent": `claude-cli/${CLAUDE_CODE_VERSION} (external, cli)`,
  "anthropic-beta":
    "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27",
} as const;

interface ClaudeUsageBucket {
  utilization?: unknown;
  resets_at?: unknown;
}

interface ParsedBucket {
  utilization?: number;
  resetsAt?: number;
}

interface ClaudeApiLimitEntry {
  kind?: unknown;
  percent?: unknown;
  resets_at?: unknown;
  scope?: unknown;
}

interface ParsedLimitEntry {
  kind: string;
  bucket: ParsedBucket;
  displayName?: string;
}

function toNumber(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const n = Number(value);
    if (Number.isFinite(n)) return n;
  }
  return undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function parseBucket(bucket: unknown): ParsedBucket | undefined {
  if (!isRecord(bucket)) return undefined;
  const utilization = toNumber(bucket.utilization);
  const resetsAt = parseIsoTimestamp(typeof bucket.resets_at === "string" ? bucket.resets_at : undefined);
  if (utilization === undefined && resetsAt === undefined) return undefined;
  return { utilization, resetsAt };
}

function displayNameOf(scope: unknown): string | undefined {
  if (!isRecord(scope)) return undefined;
  const model = scope.model;
  if (!isRecord(model)) return undefined;
  const name = model.display_name;
  return typeof name === "string" && name.trim() ? name.trim() : undefined;
}

function parseLimitEntries(raw: unknown): ParsedLimitEntry[] {
  if (!Array.isArray(raw)) return [];
  const entries: ParsedLimitEntry[] = [];
  for (const item of raw) {
    if (!isRecord(item)) continue;
    const entry = item as ClaudeApiLimitEntry;
    if (typeof entry.kind !== "string") continue;
    const utilization = toNumber(entry.percent);
    const resetsAt = parseIsoTimestamp(typeof entry.resets_at === "string" ? entry.resets_at : undefined);
    if (utilization === undefined && resetsAt === undefined) continue;
    const displayName = displayNameOf(entry.scope);
    entries.push({
      kind: entry.kind,
      bucket: { utilization, resetsAt },
      ...(displayName ? { displayName } : {}),
    });
  }
  return entries;
}

function normalizeBaseUrl(baseUrl?: string): string {
  const trimmed = baseUrl?.trim().replace(/\/+$/, "");
  if (!trimmed) return DEFAULT_ENDPOINT;
  if (trimmed.toLowerCase().endsWith("/api/oauth")) return trimmed;
  try {
    const url = new URL(trimmed);
    let path = url.pathname.replace(/\/+$/, "");
    if (path === "/") path = "";
    if (path.toLowerCase().endsWith("/v1")) path = path.slice(0, -3);
    if (!path) return `${url.origin}/api/oauth`;
    return `${url.origin}${path}/api/oauth`;
  } catch {
    return DEFAULT_ENDPOINT;
  }
}

function buildAmount(utilization: number | undefined): UsageAmount {
  if (utilization === undefined) return { unit: "percent" };
  const clamped = Math.min(Math.max(utilization, 0), 100);
  const usedFraction = clamped / 100;
  return {
    used: clamped,
    limit: 100,
    remaining: Math.max(0, 100 - clamped),
    usedFraction,
    remainingFraction: Math.max(0, 1 - usedFraction),
    unit: "percent",
  };
}

function buildStatus(usedFraction: number | undefined): UsageLimit["status"] {
  if (usedFraction === undefined) return "unknown";
  if (usedFraction >= 1) return "exhausted";
  if (usedFraction >= 0.5) return "warning";
  return "ok";
}

function buildLimit(args: {
  id: string;
  label: string;
  windowId: string;
  windowLabel: string;
  durationMs: number;
  bucket: ParsedBucket | undefined;
  provider: UsageLimit["scope"]["provider"];
  tier?: string;
  shared?: boolean;
}): UsageLimit | null {
  if (!args.bucket) return null;
  const amount = buildAmount(args.bucket.utilization);
  if (amount.used === undefined) return null;
  const window: UsageWindow = {
    id: args.windowId,
    label: args.windowLabel,
    durationMs: args.durationMs,
    ...(args.bucket.resetsAt !== undefined ? { resetsAt: args.bucket.resetsAt } : {}),
  };
  return {
    id: args.id,
    label: args.label,
    scope: {
      provider: args.provider,
      windowId: args.windowId,
      ...(args.tier !== undefined ? { tier: args.tier } : {}),
      ...(args.shared !== undefined ? { shared: args.shared } : {}),
    },
    window,
    amount,
    status: buildStatus(amount.usedFraction),
  };
}

function slugify(displayName: string): string {
  return displayName
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function parseDollarAmount(amountMinor: unknown, exponent: unknown, currency: unknown): number | undefined {
  if (
    typeof amountMinor !== "number" ||
    !Number.isSafeInteger(amountMinor) ||
    amountMinor < 0 ||
    typeof exponent !== "number" ||
    !Number.isSafeInteger(exponent) ||
    exponent < 0
  ) {
    return undefined;
  }
  if (currency !== undefined && (typeof currency !== "string" || currency.toUpperCase() !== "USD")) {
    return undefined;
  }
  const divisor = 10 ** exponent;
  if (!Number.isFinite(divisor)) return undefined;
  const dollars = amountMinor / divisor;
  return Number.isFinite(dollars) ? dollars : undefined;
}

function parseSpendExtra(value: unknown): { used: number; limit?: number } | null {
  if (!isRecord(value) || value.enabled !== true || !Object.hasOwn(value, "limit") || !isRecord(value.used)) {
    return null;
  }
  const used = parseDollarAmount(value.used.amount_minor, value.used.exponent, value.used.currency);
  if (used === undefined) return null;
  if (value.limit === null) return { used };
  if (!isRecord(value.limit)) return null;
  const limit = parseDollarAmount(value.limit.amount_minor, value.limit.exponent, value.limit.currency);
  return limit === undefined || limit <= 0 ? null : { used, limit };
}

function parseLegacyExtra(value: unknown): { used: number; limit?: number } | null {
  if (!isRecord(value) || value.is_enabled !== true || !Object.hasOwn(value, "monthly_limit")) return null;
  const decimalPlaces = value.decimal_places === undefined ? 2 : value.decimal_places;
  const used = parseDollarAmount(value.used_credits, decimalPlaces, value.currency);
  if (used === undefined) return null;
  if (value.monthly_limit === null || value.monthly_limit === undefined) return { used };
  const limit = parseDollarAmount(value.monthly_limit, decimalPlaces, value.currency);
  return limit === undefined || limit <= 0 ? null : { used, limit };
}

function buildExtraLimit(
  payload: Record<string, unknown>,
  provider: UsageLimit["scope"]["provider"],
): UsageLimit | null {
  const raw = payload.spend === null || payload.spend === undefined ? payload.extra_usage : payload.spend;
  const parsed =
    payload.spend === null || payload.spend === undefined ? parseLegacyExtra(raw) : parseSpendExtra(raw);
  if (!parsed) return null;
  if (parsed.limit === undefined) {
    return {
      id: "claude:extra",
      label: "Claude Extra Usage",
      scope: { provider, windowId: "extra" },
      amount: { used: parsed.used, unit: "usd" },
    };
  }
  const remaining = Math.max(0, parsed.limit - parsed.used);
  const usedFraction = parsed.used / parsed.limit;
  const remainingFraction = remaining / parsed.limit;
  if (!Number.isFinite(usedFraction) || !Number.isFinite(remainingFraction)) return null;
  return {
    id: "claude:extra",
    label: "Claude Extra Usage",
    scope: { provider, windowId: "extra" },
    amount: {
      used: parsed.used,
      limit: parsed.limit,
      remaining,
      usedFraction,
      remainingFraction,
      unit: "usd",
    },
    status: parsed.used >= parsed.limit ? "exhausted" : buildStatus(usedFraction),
  };
}

function parseUnifiedWindow(
  headers: Record<string, string>,
  window: "5h" | "7d" | "7d_oi",
): ParsedBucket | undefined {
  const prefix = `anthropic-ratelimit-unified-${window}-`;
  const fraction = toNumber(headers[`${prefix}utilization`]);
  const resetSeconds = toNumber(headers[`${prefix}reset`]);
  const utilization = fraction === undefined ? undefined : fraction * 100;
  const resetsAt = resetSeconds !== undefined && resetSeconds > 0 ? resetSeconds * 1000 : undefined;
  if (utilization === undefined && resetsAt === undefined) return undefined;
  return { utilization, resetsAt };
}

export function parseClaudeRateLimitHeaders(
  headers: Record<string, string>,
  now = Date.now(),
): UsageReport | null {
  const fiveHour = parseUnifiedWindow(headers, "5h");
  const sevenDay = parseUnifiedWindow(headers, "7d");
  const scoped = parseUnifiedWindow(headers, "7d_oi");
  const limits = [
    buildLimit({
      id: "claude:5h",
      label: "Claude 5 Hour",
      windowId: "5h",
      windowLabel: "5 Hour",
      durationMs: 5 * HOUR_MS,
      bucket: fiveHour,
      provider: "claude",
      shared: true,
    }),
    buildLimit({
      id: "claude:7d",
      label: "Claude 7 Day",
      windowId: "7d",
      windowLabel: "7 Day",
      durationMs: WEEK_MS,
      bucket: sevenDay,
      provider: "claude",
      shared: true,
    }),
    buildLimit({
      id: "claude:7d:fable",
      label: "Claude 7 Day (Fable)",
      windowId: "7d",
      windowLabel: "7 Day",
      durationMs: WEEK_MS,
      bucket: scoped,
      provider: "claude",
      tier: "fable",
    }),
  ].filter((limit): limit is UsageLimit => limit !== null);
  if (limits.length === 0) return null;
  return { provider: "claude", fetchedAt: now, limits, metadata: { source: "ratelimit-headers" } };
}

export const claudeUsageProvider: UsageProvider = {
  id: "claude",
  supports: params => params.provider === "claude" && params.credential.type === "oauth",
  parseRateLimitHeaders: parseClaudeRateLimitHeaders,
  async fetchUsage(params: UsageFetchParams, ctx: UsageFetchContext): Promise<UsageReport | null> {
    if (params.provider !== "claude") return null;
    const { credential } = params;
    if (credential.type !== "oauth" || !credential.accessToken) return null;

    const nowMs = Date.now();
    if (credential.expiresAt !== undefined && credential.expiresAt <= nowMs) {
      ctx.logger?.warn("Claude usage token expired", { provider: params.provider });
      return null;
    }

    const baseUrl = normalizeBaseUrl(params.baseUrl);
    const url = `${baseUrl}/usage`;
    let payload: unknown;
    try {
      const response = await ctx.fetch(url, {
        headers: { ...CLAUDE_HEADERS, authorization: `Bearer ${credential.accessToken}` },
        signal: params.signal,
      });
      if (!response.ok) {
        ctx.logger?.warn("Claude usage request failed", { status: response.status, provider: params.provider });
        return null;
      }
      payload = await response.json();
    } catch (error) {
      ctx.logger?.warn("Claude usage request error", { provider: params.provider, error: String(error) });
      return null;
    }

    if (!isRecord(payload)) return null;
    const entries = parseLimitEntries(payload.limits);
    const fiveHour = parseBucket(payload.five_hour) ?? entries.find(entry => entry.kind === "session")?.bucket;
    const sevenDay = parseBucket(payload.seven_day) ?? entries.find(entry => entry.kind === "weekly_all")?.bucket;
    const sevenDayOpus = parseBucket(payload.seven_day_opus);
    const sevenDaySonnet = parseBucket(payload.seven_day_sonnet);

    const seenSlugs = new Set<string>();
    const scoped: UsageLimit[] = [];
    for (const entry of entries) {
      if (entry.kind !== "weekly_scoped" || !entry.displayName) continue;
      const slug = slugify(entry.displayName);
      if (!slug || seenSlugs.has(slug)) continue;
      seenSlugs.add(slug);
      const limit = buildLimit({
        id: `claude:7d:${slug}`,
        label: `Claude 7 Day (${entry.displayName})`,
        windowId: "7d",
        windowLabel: "7 Day",
        durationMs: WEEK_MS,
        bucket: entry.bucket,
        provider: params.provider,
        tier: slug,
      });
      if (limit) scoped.push(limit);
    }

    const limits = [
      buildLimit({
        id: "claude:5h",
        label: "Claude 5 Hour",
        windowId: "5h",
        windowLabel: "5 Hour",
        durationMs: 5 * HOUR_MS,
        bucket: fiveHour,
        provider: params.provider,
        shared: true,
      }),
      buildLimit({
        id: "claude:7d",
        label: "Claude 7 Day",
        windowId: "7d",
        windowLabel: "7 Day",
        durationMs: WEEK_MS,
        bucket: sevenDay,
        provider: params.provider,
        shared: true,
      }),
      buildLimit({
        id: "claude:7d:opus",
        label: "Claude 7 Day (Opus)",
        windowId: "7d",
        windowLabel: "7 Day",
        durationMs: WEEK_MS,
        bucket: sevenDayOpus,
        provider: params.provider,
        tier: "opus",
      }),
      buildLimit({
        id: "claude:7d:sonnet",
        label: "Claude 7 Day (Sonnet)",
        windowId: "7d",
        windowLabel: "7 Day",
        durationMs: WEEK_MS,
        bucket: sevenDaySonnet,
        provider: params.provider,
        tier: "sonnet",
      }),
      ...scoped,
      buildExtraLimit(payload, params.provider),
    ].filter((limit): limit is UsageLimit => limit !== null);

    if (limits.length === 0) return null;
    return {
      provider: params.provider,
      fetchedAt: nowMs,
      limits,
      metadata: {
        endpoint: url,
        ...(credential.accountId ? { accountId: credential.accountId } : {}),
        ...(credential.email ? { email: credential.email } : {}),
      },
      raw: payload,
    };
  },
};
