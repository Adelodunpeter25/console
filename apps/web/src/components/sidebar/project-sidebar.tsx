import React, { useState } from "react";
import { useProjects, useAddProject, usePickNativeFolder } from "@console/api";
import { isSidebarOpen$, activeProjectId$ } from "../../state/index.js";
import { observer } from "@legendapp/state/react";
import { FolderPlus, Terminal, X } from "lucide-react";
import { SidebarListItem } from "./sidebar-list-item.js";

export const ProjectSidebar = observer(() => {
  const { data: projects = [] } = useProjects();
  const addProjectMutation = useAddProject();
  const pickNativeFolderMutation = usePickNativeFolder();
  const isOpen = isSidebarOpen$.get();
  const [expandedProjects, setExpandedProjects] = useState<Record<string, boolean>>({});

  const toggleExpand = (projectId: string) => {
    setExpandedProjects((prev) => ({
      ...prev,
      [projectId]: prev[projectId] === undefined ? false : !prev[projectId],
    }));
  };

  const handleOpenNativeFinder = async () => {
    try {
      const result = await pickNativeFolderMutation.mutateAsync();
      if (result?.path) {
        const added = await addProjectMutation.mutateAsync(result.path);
        activeProjectId$.set(added.id);
      }
    } catch {
      // User cancelled native Finder dialog
    }
  };

  if (!isOpen) return null;

  return (
    <aside className="w-72 h-screen sidebar-surface flex flex-col z-30 shrink-0 select-none">
      {/* Title Header */}
      <div className="h-12 px-3 border-b border-border/40 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="w-6 h-6 rounded-md bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
            <Terminal size={14} />
          </div>
          <span className="font-semibold text-sm tracking-tight text-foreground">Console</span>
        </div>
        <button
          onClick={() => isSidebarOpen$.set(false)}
          className="p-1 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground cursor-pointer transition-colors"
        >
          <X size={15} />
        </button>
      </div>

      {/* Projects Navigation Header */}
      <div className="p-3 border-b border-border/30 flex items-center justify-between">
        <span className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
          Projects
        </span>
        <button
          onClick={handleOpenNativeFinder}
          disabled={pickNativeFolderMutation.isPending}
          className="p-1 rounded hover:bg-accent text-primary flex items-center gap-1 text-[11px] font-medium cursor-pointer disabled:opacity-50"
        >
          <FolderPlus size={13} /> {pickNativeFolderMutation.isPending ? "Opening Finder..." : "Add Project"}
        </button>
      </div>

      {/* Project Tree Items */}
      <div className="flex-1 overflow-y-auto p-2 flex flex-col gap-1">
        {projects.length === 0 ? (
          <div className="p-4 text-center text-xs text-muted-foreground">
            No projects added yet. Click "+ Add Project" to select a folder.
          </div>
        ) : (
          projects.map((project) => (
            <SidebarListItem
              key={project.id}
              project={project}
              isExpanded={expandedProjects[project.id] ?? true}
              onToggleExpand={() => toggleExpand(project.id)}
            />
          ))
        )}
      </div>
    </aside>
  );
});
