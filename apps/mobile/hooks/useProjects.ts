import { useCallback, useEffect, useRef } from "react";
import { useProjectStore } from "../stores/useProjectStore";

/** Store-backed projects list + add-project. */
export function useProjects() {
  const projects = useProjectStore((state) => state.projects);
  const loading = useProjectStore((state) => state.loading);
  const loadProjects = useProjectStore((state) => state.loadProjects);
  const addProject = useProjectStore((state) => state.addProject);

  // Auto-load once on mount. Guarded so an empty result (or a failed fetch)
  // can't re-trigger the effect forever, which otherwise recreates the
  // projects array reference on every render and loops dependents.
  const autoLoadStarted = useRef(false);
  useEffect(() => {
    if (autoLoadStarted.current) return;
    if (projects.length === 0 && !loading) {
      autoLoadStarted.current = true;
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
