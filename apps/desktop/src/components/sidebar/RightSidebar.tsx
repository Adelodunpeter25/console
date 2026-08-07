import React from "react";
import { FolderTree, RefreshCw } from "lucide-react";
import { useAppStore } from "../../store/useAppStore";
import { useProjectStore } from "../../store/useProjectStore";
import { useFsStore } from "../../store/useFsStore";
import { FileTree } from "../file/FileTree";

export function RightSidebar({ width = 288 }: { width?: number }) {
  const selectedProjectId = useAppStore((state) => state.selectedProjectId);
  const projects = useProjectStore((state) => state.projects);
  const { browse, browseDirectory } = useFsStore();

  const currentProject = React.useMemo(
    () => projects.find((p) => p.id === selectedProjectId),
    [projects, selectedProjectId],
  );

  const projectPath = currentProject?.path;

  React.useEffect(() => {
    if (projectPath) {
      browseDirectory(projectPath).catch(() => {});
    }
  }, [projectPath, browseDirectory]);

  const tree = browse && browse.path === projectPath ? browse.entries : [];

  const handleRefresh = () => {
    if (projectPath) {
      browseDirectory(projectPath).catch(() => {});
    }
  };

  return (
    <aside
      style={{ width }}
      className="flex flex-col h-full bg-surface border-l border-border select-none overflow-hidden shrink-0"
    >
      <div className="flex items-center justify-between px-3 py-2 border-b border-border">
        <div className="flex items-center gap-2 text-foreground font-medium text-xs">
          <FolderTree size={14} className="text-primary-light" />
          <span className="truncate">{currentProject?.name ?? "Explorer"}</span>
        </div>
        {projectPath && (
          <button
            onClick={handleRefresh}
            title="Refresh File Tree"
            className="p-1 text-foreground-muted hover:text-foreground hover:bg-surface-hover rounded transition-colors"
          >
            <RefreshCw size={12} />
          </button>
        )}
      </div>

      <div className="flex-1 overflow-y-auto">
        {projectPath ? (
          <FileTree tree={tree} />
        ) : (
          <div className="p-4 text-xs text-foreground-muted text-center">
            No active project selected.
          </div>
        )}
      </div>
    </aside>
  );
}
