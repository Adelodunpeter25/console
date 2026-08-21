import { useQuery } from "@tanstack/react-query";
import type { ProjectInfo } from "@console/types";
import { gitService } from "@console/api";

/** Stable key derived from the projects so the query only re-fetches when the actual set of projects changes. */
function projectKey(projects: ProjectInfo[]) {
  return projects.map((p) => `${p.id}:${p.path}`).join("|");
}

async function fetchBranches(projects: ProjectInfo[]): Promise<Record<string, string>> {
  if (projects.length === 0) return {};
  const results = await Promise.all(
    projects.map(async (project) => {
      try {
        const summary = await gitService.getStatus(project.path);
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
 * Cached by TanStack Query with a 60s stale time to avoid repetitive git processes.
 */
export function useProjectBranches(projects: ProjectInfo[]) {
  return useQuery({
    queryKey: ["git", "branches", projectKey(projects)],
    queryFn: () => fetchBranches(projects),
    enabled: projects.length > 0,
    staleTime: 60_000,
    placeholderData: (previous) => previous ?? {},
  });
}