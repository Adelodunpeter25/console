import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { sessionService, fsService, sessionKeys, fsKeys } from "@console/api";
import type { CreateSessionDto, UpdateSessionDto } from "@console/types";

export { sessionKeys, fsKeys };

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

export function prefetchSession(queryClient: ReturnType<typeof useQueryClient>, id: string) {
  if (!id) return;
  void queryClient.prefetchQuery({
    queryKey: sessionKeys.detail(id),
    queryFn: () => sessionService.getSession(id),
    staleTime: 60_000,
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

export function useProjects() {
  return useQuery({
    queryKey: fsKeys.projects,
    queryFn: () => fsService.getProjects(),
    staleTime: 15_000,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
  });
}

export function useAddProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (path: string) => fsService.addProject(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: fsKeys.projects });
    },
  });
}

export function useDeleteProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => fsService.deleteProject(projectId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: fsKeys.projects });
    },
  });
}

export function useFsBrowse(path?: string) {
  return useQuery({
    queryKey: fsKeys.browse(path),
    queryFn: () => fsService.getFsBrowse(path),
  });
}

export function useFsTree(path?: string) {
  return useQuery({
    queryKey: fsKeys.tree(path),
    queryFn: () => fsService.getFsTree(path),
  });
}

export function useReadFile(path: string) {
  return useQuery({
    queryKey: fsKeys.file(path),
    queryFn: () => fsService.readFile(path),
    enabled: Boolean(path),
  });
}
