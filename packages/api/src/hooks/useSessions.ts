import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { sessionService } from "../services/session.service";
import type { CreateSessionDto, UpdateSessionDto } from "@console/types";

export const sessionKeys = {
  all: ["sessions"] as const,
  lists: (params?: { cwd?: string; projectId?: string; onlyDeleted?: boolean }) =>
    [...sessionKeys.all, "list", params] as const,
  detail: (id: string) => [...sessionKeys.all, "detail", id] as const,
};

export function useSessions(params?: { cwd?: string; projectId?: string; onlyDeleted?: boolean }) {
  return useQuery({
    queryKey: sessionKeys.lists(params),
    queryFn: () => sessionService.getSessions(params),
    staleTime: 15_000,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
  });
}

export function useSession(id: string) {
  return useQuery({
    queryKey: sessionKeys.detail(id),
    queryFn: () => sessionService.getSession(id),
    enabled: Boolean(id),
    staleTime: 15_000,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
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
    onSuccess: () => {
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

export function useRestoreSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => sessionService.restoreSession(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.all });
    },
  });
}

export function usePermanentlyDeleteSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => sessionService.permanentlyDeleteSession(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.all });
    },
  });
}
