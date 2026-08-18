import { useCallback, useEffect, useState } from "react";
import type { GitStatusSummary, ProjectInfo } from "@console/types";
import { getConsoleApiClient } from "@console/api";

/**
 * Fetch the current git branch for every project path in one pass.
 * Returns a map of projectId -> branch (empty when the folder isn't a git repo).
 */
export function useProjectBranches(projects: ProjectInfo[]) {
  const [branches, setBranches] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    if (projects.length === 0) {
      setBranches({});
      return;
    }
    setLoading(true);
    try {
      const results = await Promise.all(
        projects.map(async (project) => {
          try {
            const res = await getConsoleApiClient().get("/api/git/status", {
              params: { path: project.path },
            });
            const summary: GitStatusSummary = res.data?.data;
            return [project.id, summary?.branch ?? ""] as const;
          } catch {
            return [project.id, ""] as const;
          }
        }),
      );
      setBranches(Object.fromEntries(results));
    } catch {
      setBranches({});
    } finally {
      setLoading(false);
    }
  }, [projects]);

  useEffect(() => {
    load();
  }, [load]);

  return { branches, loading, refetch: load };
}