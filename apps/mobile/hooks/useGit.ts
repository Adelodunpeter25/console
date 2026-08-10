import { useCallback, useEffect, useState } from "react";
import type { GitStatusSummary } from "@console/types";
import { getConsoleApiClient } from "@console/api";

/**
 * Fetch the git status summary for a path from the backend
 * (GET /api/git/status).
 */
export function useGitStatus(path?: string | null) {
  const [summary, setSummary] = useState<GitStatusSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!path) {
      setSummary(null);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const res = await getConsoleApiClient().get("/api/git/status", { params: { path } });
      setSummary(res.data?.data ?? null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load git status");
      setSummary(null);
    } finally {
      setLoading(false);
    }
  }, [path]);

  useEffect(() => {
    load();
  }, [load]);

  return { summary, loading, error, refetch: load };
}
