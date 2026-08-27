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
  googleGeminiCliUsageProvider,
  antigravityUsageProvider,
  openaiCodexUsageProvider,
} from "@/providers/src/usage/index.js";
import type { UsageFetchParams } from "@console/types";

const CACHE_TTL_MS = 60_000;

interface CacheEntry {
  report: UsageReport | null;
  fetchedAt: number;
}

export class UsageService {
  private cache = new Map<ProviderId, CacheEntry>();

  private isCacheValid(entry: CacheEntry | undefined): boolean {
    if (!entry) return false;
    return Date.now() - entry.fetchedAt < CACHE_TTL_MS;
  }

  private getUsageProvider(provider: ProviderId) {
    switch (provider) {
      case "gemini":
        return googleGeminiCliUsageProvider;
      case "antigravity":
        return antigravityUsageProvider;
      case "codex":
        return openaiCodexUsageProvider;
      default:
        return null;
    }
  }

  async getUsage(provider: ProviderId, signal?: AbortSignal): Promise<UsageReport | null> {
    if (provider !== "gemini" && provider !== "antigravity" && provider !== "codex") {
      return null;
    }

    const cached = this.cache.get(provider);
    if (this.isCacheValid(cached)) {
      return cached!.report;
    }

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
        const raw = await loadCredential(provider);
        const cred = await refreshIfNeeded(raw, provider, signal);
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
      const report = null;
      this.cache.set(provider, { report, fetchedAt: Date.now() });
      return report;
    }

    if (!credential) {
      this.cache.set(provider, { report: null, fetchedAt: Date.now() });
      return null;
    }

    const params: UsageFetchParams = {
      provider,
      credential,
      signal,
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
      // TTL for null reports is shorter to retry auth failures sooner
      return report;
    } catch (err) {
      console.warn(`[usage:${provider}] fetch failed`, err);
      // Don't cache failures long — let next request retry
      return null;
    }
  }

  async getAllUsage(signal?: AbortSignal): Promise<Record<string, UsageReport | null>> {
    const providers: ProviderId[] = ["gemini", "antigravity", "codex"];
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
