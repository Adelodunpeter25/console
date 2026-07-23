import React, { useState } from "react";
import { useProjects, useAddProject } from "@console/api";
import { isSidebarOpen$, activeProjectId$ } from "../../state/index.js";
import { observer } from "@legendapp/state/react";
import { FolderPlus, Terminal, X } from "lucide-react";
import { DirectoryPickerModal } from "../common/directory-picker-modal.js";
import { SidebarListItem } from "./sidebar-list-item.js";
import { ChatList } from "../chat/chat-list.js";

export const ProjectSidebar = observer(() => {
  const { data: projects = [] } = useProjects();
  const addProjectMutation = useAddProject();
  const isOpen = isSidebarOpen$.get();
  const [showDirPicker, setShowDirPicker] = useState(false);
  const [expandedProjects, setExpandedProjects] = useState<Record<string, boolean>>({});

  const toggleExpand = (projectId: string) => {
    setExpandedProjects((prev) => ({
      ...prev,
      [projectId]: !prev[projectId],
    }));
  };

  const handleSelectDirectory = async (path: string) => {
    try {
      const added = await addProjectMutation.mutateAsync(path);
      activeProjectId$.set(added.id);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to add project");
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
          onClick={() => setShowDirPicker(true)}
          className="p-1 rounded hover:bg-accent text-primary flex items-center gap-1 text-[11px] font-medium cursor-pointer"
        >
          <FolderPlus size={13} /> Add Project
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
              isExpanded={Boolean(expandedProjects[project.id])}
              onToggleExpand={() => toggleExpand(project.id)}
            />
          ))
        )}
      </div>

      {/* Searchable Chat List */}
      <ChatList />

      {/* Directory Picker Modal */}
      <DirectoryPickerModal
        isOpen={showDirPicker}
        onClose={() => setShowDirPicker(false)}
        onSelect={handleSelectDirectory}
      />
    </aside>
  );
});
