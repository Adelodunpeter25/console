import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { sessionService } from "../services/session.service";
import type { CreateSessionDto, UpdateSessionDto } from "@console/types";

export const sessionKeys = {
  all: ["sessions"] as const,
  lists: (params?: { cwd?: string; projectId?: string }) =>
    [...sessionKeys.all, "list", params] as const,
  detail: (id: string) => [...sessionKeys.all, "detail", id] as const,
};

export function useSessions(params?: { cwd?: string; projectId?: string }) {
  return useQuery({
    queryKey: sessionKeys.lists(params),
    queryFn: () => sessionService.getSessions(params),
  });
}

export function useSession(id: string) {
  return useQuery({
    queryKey: sessionKeys.detail(id),
    queryFn: () => sessionService.getSession(id),
    enabled: Boolean(id),
  });
}

export function useCreateSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateSessionDto) => sessionService.createSession(payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.all });
    },
  });
}

export function useUpdateSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateSessionDto }) =>
      sessionService.updateSession(id, payload),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.all });
    },
  });
}

export function useDeleteSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => sessionService.deleteSession(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.all });
    },
  });
}
