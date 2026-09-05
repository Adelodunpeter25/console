import { useCallback, useEffect, useState } from "react";
import type { GitStatusSummary } from "@console/types";
import { gitService } from "@console/api";
import { app$ } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";

/**
 * Git status summary for a path.
 * Prefers the live SSE stream (GET /api/git/status/watch) with a one-shot
 * GET fallback when the stream errors or closes. Pass { watch: false } for
 * the legacy single fetch.
 */
export function useGitStatus(path?: string | null, options?: { watch?: boolean }) {
  const watch = options?.watch ?? true;
  const backendUrl = useValue(app$.backendUrl);
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
      setSummary(await gitService.getStatus(path));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load git status");
      setSummary(null);
    } finally {
      setLoading(false);
    }
  }, [path]);

  useEffect(() => {
    if (!path) {
      setSummary(null);
      return;
    }
    if (!watch || !backendUrl) {
      load();
      return;
    }
    setLoading(true);
    setError(null);
    let cancelled = false;
    const stop = gitService.watchStatus(
      backendUrl,
      path,
      (next) => {
        if (cancelled) return;
        setSummary(next);
        setLoading(false);
      },
      () => {
        // Stream failed (server without the watch route?) — fall back once.
        if (!cancelled) load();
      },
    );
    return () => {
      cancelled = true;
      stop();
    };
  }, [path, watch, backendUrl, load]);

  return { summary, loading, error, refetch: load };
}
