/**
 * Usage Quota Service — aggregates per-provider quota via UsageProviders.
 * Caches reports for 60s to avoid hammering upstream.
 */
import type { ProviderId } from "@console/types";
import type { UsageReport } from "@console/types";
import { loadCredential } from "@/providers/src/auth/token-store.js";
import { refreshIfNeeded } from "@/providers/src/auth/token-refresh.js";
import { loadCodexCredential, refreshCodexIfNeeded } from "@/providers/src/codex/oauth.js";
import {
  antigravityUsageProvider,
  openaiCodexUsageProvider,
} from "@/providers/src/usage/index.js";
import type { UsageFetchParams } from "@console/types";

const CACHE_TTL_MS = 60_000;
/** Hard upstream cap per provider — prevents slow APIs from blocking the entire /api/usage response. */
const UPSTREAM_TIMEOUT_MS = 5_000;

interface CacheEntry {
  report: UsageReport | null;
  fetchedAt: number;
}

export class UsageService {
  private cache = new Map<ProviderId, CacheEntry>();
  /** In-flight deduplication: prevents multiple concurrent requests for the same provider. */
  private inflight = new Map<ProviderId, Promise<UsageReport | null>>();

  private isCacheValid(entry: CacheEntry | undefined): boolean {
    if (!entry) return false;
    return Date.now() - entry.fetchedAt < CACHE_TTL_MS;
  }

  private getUsageProvider(provider: ProviderId) {
    switch (provider) {
      case "antigravity":
        return antigravityUsageProvider;
      case "codex":
        return openaiCodexUsageProvider;
      default:
        return null;
    }
  }

  async getUsage(provider: ProviderId, signal?: AbortSignal): Promise<UsageReport | null> {
    if (provider !== "antigravity" && provider !== "codex") {
      return null;
    }

    const cached = this.cache.get(provider);
    if (this.isCacheValid(cached)) {
      return cached!.report;
    }

    // Return in-flight promise directly if a request for this provider is already underway.
    const existing = this.inflight.get(provider);
    if (existing) return existing;

    const promise = this._fetchUsage(provider, signal).finally(() => {
      this.inflight.delete(provider);
    });
    this.inflight.set(provider, promise);
    return promise;
  }

  private async _fetchUsage(provider: ProviderId, signal?: AbortSignal): Promise<UsageReport | null> {
    const usageProvider = this.getUsageProvider(provider);
    if (!usageProvider) return null;

    let credential: UsageFetchParams["credential"] | null = null;

    try {
      if (provider === "codex") {
        const raw = await loadCodexCredential();
        const cred = await refreshCodexIfNeeded(raw);
        credential = {
          type: "oauth",
          accessToken: cred.accessToken,
          expiresAt: cred.expiresAtMs,
          accountId: cred.accountId,
          email: cred.email,
        };
      } else {
        const raw = await loadCredential(provider as import("@console/types").OAuthProviderId);
        const cred = await refreshIfNeeded(raw, provider as import("@console/types").OAuthProviderId, signal);
        credential = {
          type: "oauth",
          accessToken: cred.accessToken,
          expiresAt: cred.expiresAtMs,
          projectId: cred.projectId,
          email: cred.email,
        };
      }
    } catch {
      // Not logged in or credential missing — cache null briefly to avoid tight loop
      this.cache.set(provider, { report: null, fetchedAt: Date.now() });
      return null;
    }

    if (!credential) {
      this.cache.set(provider, { report: null, fetchedAt: Date.now() });
      return null;
    }

    // Compose the caller signal with a per-provider hard timeout so a slow upstream
    // (e.g. Gemini quota API taking 4s) never stalls the whole /api/usage response.
    const timeoutController = new AbortController();
    const timeoutId = setTimeout(() => timeoutController.abort(), UPSTREAM_TIMEOUT_MS);
    const composedSignal = signal
      ? AbortSignal.any([signal, timeoutController.signal])
      : timeoutController.signal;

    const params: UsageFetchParams = {
      provider,
      credential,
      signal: composedSignal,
    };

    try {
      const report = await usageProvider.fetchUsage(params, {
        fetch: fetch as unknown as typeof globalThis.fetch,
        logger: {
          debug: () => {},
          warn: (msg, meta) => console.warn(`[usage:${provider}] ${msg}`, meta),
        },
      });

      this.cache.set(provider, { report, fetchedAt: Date.now() });
      return report;
    } catch (err) {
      console.warn(`[usage:${provider}] fetch failed`, err);
      return null;
    } finally {
      clearTimeout(timeoutId);
    }
  }

  async getAllUsage(signal?: AbortSignal): Promise<Record<string, UsageReport | null>> {
    const providers: ProviderId[] = ["antigravity", "codex"];
    const results = await Promise.all(
      providers.map(async (p) => {
        const report = await this.getUsage(p, signal);
        return [p, report] as const;
      }),
    );
    return Object.fromEntries(results) as Record<string, UsageReport | null>;
  }

  invalidate(provider?: ProviderId): void {
    if (provider) this.cache.delete(provider);
    else this.cache.clear();
  }
}
