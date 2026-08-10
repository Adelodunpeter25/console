import { useCallback, useEffect } from "react";
import { useProjectStore } from "../stores/useProjectStore";

/** Store-backed projects list + add-project. */
export function useProjects() {
  const projects = useProjectStore((state) => state.projects);
  const loading = useProjectStore((state) => state.loading);
  const loadProjects = useProjectStore((state) => state.loadProjects);
  const addProject = useProjectStore((state) => state.addProject);

  useEffect(() => {
    if (projects.length === 0 && !loading) {
      loadProjects().catch(() => {});
    }
  }, [projects.length, loading, loadProjects]);

  const handleAdd = useCallback(
    async (path: string) => {
      return addProject(path);
    },
    [addProject],
  );

  return { data: projects, isLoading: loading, refetch: loadProjects, addProject: handleAdd };
}
