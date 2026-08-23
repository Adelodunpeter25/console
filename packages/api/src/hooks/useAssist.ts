import { useQuery } from "@tanstack/react-query";
import { assistService } from "../services/assist.service";

export const assistKeys = {
  commands: (sessionId?: string | null) => ["assist", "commands", sessionId ?? "global"] as const,
  search: (sessionId: string | null | undefined, query: string, root?: string) =>
    ["assist", "search", sessionId ?? "global", query, root ?? ""] as const,
};

export function useSlashCommands(sessionId?: string | null) {
  return useQuery({
    queryKey: assistKeys.commands(sessionId),
    queryFn: () => assistService.listSlashCommands(sessionId),
    enabled: true,
    staleTime: 60_000,
  });
}

export function useFileSearch(sessionId: string | null | undefined, query: string, root?: string) {
  return useQuery({
    queryKey: assistKeys.search(sessionId, query, root),
    queryFn: () => assistService.searchFiles(sessionId, query, root),
    enabled: Boolean(query && query.trim().length > 0),
    staleTime: 15_000,
  });
}
