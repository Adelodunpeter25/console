import { useEffect } from "react";
import { useValue } from "@legendapp/state/react";
import {
  loadAllUsage,
  loadUsage,
  usage$,
} from "@/stores/useUsageStore";

/**
 * Usage quota hooks — thin wrappers over `usage$` Legend State.
 * Follows `hooks/useProviders.ts:6` + `hooks/useAuth.ts:22` pattern.
 *
 * Components subscribe via `useValue`; stores own fetching + caching.
 * No TanStack Query here — the server already caches 60s and the store
 * throttles via `lastFetchedAt` (30s stale window in the hook).
 */

const STALE_MS = 30_000;

export function useAllUsage() {
  const reports = useValue(usage$.reports);
  const loading = useValue(usage$.loading);
  const error = useValue(usage$.error);
  const lastFetchedAt = useValue(usage$.lastFetchedAt);

  useEffect(() => {
    const isStale = !lastFetchedAt || Date.now() - lastFetchedAt > STALE_MS;
    if (!loading && isStale) {
      void loadAllUsage().catch(() => {});
    }
  }, [loading, lastFetchedAt]);

  return {
    data: reports as Record<string, import("@console/types").UsageReport | null>,
    isLoading: loading && Object.keys(reports).length === 0,
    isRefetching: loading,
    error,
    refetch: loadAllUsage,
  };
}

export function useUsage(providerId: string) {
  const report = useValue(() => usage$.reports[providerId].get());
  const loading = useValue(() => Boolean(usage$.loadingByProvider[providerId].get()));
  const error = useValue(usage$.error);

  useEffect(() => {
    if (!providerId) return;
    const isStale = !usage$.lastFetchedAt.peek() || Date.now() - (usage$.lastFetchedAt.peek() ?? 0) > STALE_MS;
    if (report === undefined && !loading) {
      void loadUsage(providerId).catch(() => {});
    } else if (isStale && !loading) {
      void loadUsage(providerId).catch(() => {});
    }
  }, [providerId, report, loading]);

  return {
    data: report as import("@console/types").UsageReport | null | undefined,
    isLoading: loading && report === undefined,
    error,
    refetch: () => loadUsage(providerId),
  };
}
