import { batch, observable } from "@legendapp/state";
import type { UsageReport } from "@console/types";
import { usageService } from "@console/api";

/**
 * Usage quota state as Legend State observables.
 * Mirrors `useProviderStore` / `useAuthStore` pattern.
 *
 * Reads via `useValue(usage$.field)`; imperative reads via `.peek()`.
 * Server caches 60s (`apps/server/api/src/services/usage.service.ts:8`), so client
 * throttles to avoid tight polling — 30s staleTime in the hook layer.
 */
export const usage$ = observable({
  /** Reports keyed by provider id (antigravity/codex), null = not logged in / unavailable. */
  reports: {} as Record<string, UsageReport | null>,
  loading: false,
  loadingByProvider: {} as Record<string, boolean>,
  error: null as string | null,
  lastFetchedAt: null as number | null,
});

export async function loadAllUsage(): Promise<Record<string, UsageReport | null>> {
  batch(() => {
    usage$.loading.set(true);
    usage$.error.set(null);
  });
  try {
    const reports = await usageService.getAllUsage();
    batch(() => {
      for (const [provider, report] of Object.entries(reports)) {
        usage$.reports[provider].set(report as UsageReport | null);
        usage$.loadingByProvider[provider].set(false);
      }
      usage$.loading.set(false);
      usage$.lastFetchedAt.set(Date.now());
    });
    return reports as Record<string, UsageReport | null>;
  } catch (e) {
    batch(() => {
      usage$.loading.set(false);
      usage$.error.set(e instanceof Error ? e.message : "Failed to load usage");
    });
    throw e;
  }
}

export async function loadUsage(providerId: string): Promise<UsageReport | null> {
  const cached = usage$.reports[providerId].peek() as UsageReport | null | undefined;
  // Return cached if we have it and not currently loading — hook layer handles staleness.
  if (cached !== undefined && !usage$.loadingByProvider[providerId].peek()) {
    // Still refresh in background if stale? Let hook decide; here just return.
  }

  batch(() => {
    usage$.loadingByProvider[providerId].set(true);
    usage$.error.set(null);
  });
  try {
    const report = await usageService.getProviderUsage(providerId);
    batch(() => {
      usage$.reports[providerId].set(report);
      usage$.loadingByProvider[providerId].set(false);
      usage$.lastFetchedAt.set(Date.now());
    });
    return report;
  } catch (e) {
    batch(() => {
      usage$.loadingByProvider[providerId].set(false);
      usage$.error.set(e instanceof Error ? e.message : `Failed to load usage for ${providerId}`);
    });
    throw e;
  }
}

export function clearUsageError(): void {
  usage$.error.set(null);
}

export function invalidateUsage(providerId?: string): void {
  if (providerId) {
    usage$.reports[providerId].delete();
    usage$.loadingByProvider[providerId].set(false);
  } else {
    usage$.reports.set({} as Record<string, UsageReport | null>);
    usage$.loadingByProvider.set({} as Record<string, boolean>);
    usage$.lastFetchedAt.set(null);
  }
}
