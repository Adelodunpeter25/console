import { useQuery } from "@tanstack/react-query";
import { gitService } from "../services/git.service";

export const gitKeys = {
  status: (path?: string | null) => ["git", "status", path || "root"] as const,
};

export function useGitStatus(path?: string | null) {
  return useQuery({
    queryKey: gitKeys.status(path),
    queryFn: () => gitService.getStatus(path ?? ""),
    enabled: Boolean(path),
  });
}