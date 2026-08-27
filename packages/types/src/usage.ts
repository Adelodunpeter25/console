/**
 * Usage reporting types for provider quota/limit endpoints.
 * Ported from oh-my-pi/packages/ai/src/usage.ts — trimmed for console.
 */
import type { ProviderId } from "./model";

export type UsageUnit = "percent" | "tokens" | "requests" | "usd" | "minutes" | "bytes" | "unknown";
export type UsageStatus = "ok" | "warning" | "exhausted" | "unknown";

export interface UsageWindow {
  id: string;
  label: string;
  durationMs?: number;
  resetsAt?: number;
  resetLabel?: string;
}

export interface UsageAmount {
  used?: number;
  limit?: number;
  remaining?: number;
  usedFraction?: number;
  remainingFraction?: number;
  unit: UsageUnit;
}

export interface UsageScope {
  provider: ProviderId;
  accountId?: string;
  projectId?: string;
  orgId?: string;
  modelId?: string;
  tier?: string;
  windowId?: string;
  shared?: boolean;
}

export interface UsageLimit {
  id: string;
  label: string;
  scope: UsageScope;
  window?: UsageWindow;
  amount: UsageAmount;
  status?: UsageStatus;
  notes?: string[];
}

export interface UsageResetCreditDetail {
  grantedAt?: string;
  expiresAt?: string;
  status?: string;
}

export interface UsageResetCredits {
  availableCount: number;
  credits?: UsageResetCreditDetail[];
}

export interface UsageReport {
  provider: ProviderId;
  fetchedAt: number;
  limits: UsageLimit[];
  resetCredits?: UsageResetCredits;
  notes?: string[];
  metadata?: Record<string, unknown>;
  raw?: unknown;
}

export function resolveUsedFraction(limit: UsageLimit): number | undefined {
  const amount = limit.amount;
  if (amount.usedFraction !== undefined) return amount.usedFraction;
  if (amount.used !== undefined && amount.limit !== undefined && amount.limit > 0) {
    return amount.used / amount.limit;
  }
  if (amount.unit === "percent" && amount.used !== undefined) return amount.used / 100;
  if (amount.remainingFraction !== undefined) return Math.max(0, 1 - amount.remainingFraction);
  return undefined;
}

export interface UsageLogger {
  debug(message: string, meta?: Record<string, unknown>): void;
  warn(message: string, meta?: Record<string, unknown>): void;
}

export interface UsageCredential {
  type: "api_key" | "oauth";
  apiKey?: string;
  accessToken?: string;
  refreshToken?: string;
  expiresAt?: number;
  accountId?: string;
  projectId?: string;
  email?: string;
  orgId?: string;
  orgName?: string;
  enterpriseUrl?: string;
  metadata?: Record<string, unknown>;
  apiEndpoint?: string;
}

export interface UsageFetchParams {
  provider: ProviderId;
  credential: UsageCredential;
  accountKey?: string;
  baseUrl?: string;
  signal?: AbortSignal;
}

export type FetchImpl = typeof fetch;

export interface UsageFetchContext {
  fetch: FetchImpl;
  logger?: UsageLogger;
  retryWait?: (delayMs: number, signal?: AbortSignal) => Promise<void>;
}

export interface UsageProvider {
  id: ProviderId;
  fetchUsage(params: UsageFetchParams, ctx: UsageFetchContext): Promise<UsageReport | null>;
  parseRateLimitHeaders?(headers: Record<string, string>, now?: number): UsageReport | null;
  supports?(params: UsageFetchParams): boolean;
  validatesCredentials?: boolean;
  retainLastGoodOnFailure?: boolean;
}

export interface CredentialRankingContext {
  modelId?: string;
}

export interface CredentialRankingStrategy {
  findWindowLimits(
    report: UsageReport,
    context?: CredentialRankingContext,
  ): {
    primary?: UsageLimit;
    secondary?: UsageLimit;
  };
  scopeLimits?(report: UsageReport, context?: CredentialRankingContext): UsageLimit[];
  scopeLimitsForReserve?(report: UsageReport, context?: CredentialRankingContext): UsageLimit[];
  blockScope?(context?: CredentialRankingContext): string | undefined;
  blockScopes?(context?: CredentialRankingContext): string[];
  windowDefaults: {
    primaryMs: number;
    secondaryMs: number;
  };
  hasPriorityBoost?(primary: UsageLimit | undefined): boolean;
}
