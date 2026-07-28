import React from "react";
import { FolderOpen } from "lucide-react";
import { useAppStore, useProjectStore } from "../../store";
import { ProjectSection } from "./ProjectSection";

/**
 * Left sidebar — project navigator.
 *
 * Renders nothing when collapsed (the toggle button lives in the TitleBar).
 * When open, shows a simple header and the project list.
 */
export function Sidebar() {
  const { sidebarOpen, expandedProjects } = useAppStore();
  const { projects, loading, loadProjects, sessionsByProject } = useProjectStore();

  React.useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  if (!sidebarOpen) return null;

  return (
    <div className="w-72 bg-sidebar border-r border-border flex flex-col h-full shrink-0">
      {/* Top bar */}
      <div className="px-4 h-12 flex items-center border-b border-border shrink-0">
        <span className="text-sm font-bold text-foreground tracking-tight">Projects</span>
      </div>

      {/* Project sections */}
      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <span className="text-sm text-foreground-muted">Loading...</span>
          </div>
        ) : projects.length === 0 ? (
          <div className="px-2 py-8 text-center">
            <FolderOpen size={28} className="mx-auto text-foreground-muted mb-2" />
            <p className="text-xs text-foreground-muted">
              No projects yet.
            </p>
          </div>
        ) : (
          projects.map((project) => (
            <ProjectSection
              key={project.id}
              project={project}
              sessions={sessionsByProject[project.id] ?? []}
              isExpanded={expandedProjects.has(project.id)}
            />
          ))
        )}
      </div>
    </div>
  );
}
