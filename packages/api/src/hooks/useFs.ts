import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fsService } from "../services/fs.service";

export const fsKeys = {
  projects: ["projects"] as const,
  browse: (path?: string) => ["fs", "browse", path || "root"] as const,
  tree: (path?: string) => ["fs", "tree", path || "root"] as const,
  file: (path: string) => ["fs", "file", path] as const,
};

export function useProjects() {
  return useQuery({
    queryKey: fsKeys.projects,
    queryFn: () => fsService.getProjects(),
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

export function usePickNativeFolder() {
  return useMutation({
    mutationFn: () => fsService.pickNativeFolder(),
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

export function useWriteFile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ path, content }: { path: string; content: string }) =>
      fsService.writeFile(path, content),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: fsKeys.file(variables.path) });
      queryClient.invalidateQueries({ queryKey: ["fs"] });
    },
  });
}

export function useDeleteFile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (path: string) => fsService.deleteFile(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fs"] });
    },
  });
}

export function useCreateDir() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (path: string) => fsService.createDir(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fs"] });
    },
  });
}

export function useDeleteDir() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (path: string) => fsService.deleteDir(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fs"] });
    },
  });
}
