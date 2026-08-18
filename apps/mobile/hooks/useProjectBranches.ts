import { useQuery } from "@tanstack/react-query";
import type { GitStatusSummary, ProjectInfo } from "@console/types";
import { getConsoleApiClient } from "@console/api";

/** Stable key derived from the projects so the query only re-fetches when the actual set of projects changes. */
function projectKey(projects: ProjectInfo[]) {
  return projects.map((p) => `${p.id}:${p.path}`).join("|");
}

async function fetchBranches(projects: ProjectInfo[]): Promise<Record<string, string>> {
  if (projects.length === 0) return {};
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
  return Object.fromEntries(results);
}

/**
 * Fetch the current git branch for every project path in one pass.
 * Returns a map of projectId -> branch (empty when the folder isn't a git repo).
 * Cached by TanStack Query; refetch on demand via `refetch`.
 */
export function useProjectBranches(projects: ProjectInfo[]) {
  return useQuery({
    queryKey: ["git", "branches", projectKey(projects)],
    queryFn: () => fetchBranches(projects),
    placeholderData: (previous) => previous ?? {},
  });
}