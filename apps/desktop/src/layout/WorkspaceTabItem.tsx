import React from "react";
import { MessageSquare, Terminal, X } from "lucide-react";
import { WorkspaceTabConfig, getTabTitle } from "./types";
import { FileIcon } from "../components/file/FileIcon";
import { useWorkspaceStore } from "./useWorkspaceStore";

interface WorkspaceTabItemProps {
  paneId?: string;
  config: WorkspaceTabConfig;
  isActive: boolean;
  onSelect: () => void;
  onClose: () => void;
}

function getTabIcon(config: WorkspaceTabConfig) {
  const iconClass = "text-foreground-muted shrink-0";
  switch (config.type) {
    case "chat":
      return <MessageSquare size={13} className={iconClass} />;
    case "terminal":
      return <Terminal size={13} className={iconClass} />;
    case "file":
    case "diff":
      return <FileIcon fileName={config.path} className="w-3.5 h-3.5 shrink-0 text-foreground-muted" />;
  }
}

/**
 * WorkspaceTabItem — Draggable tab pill using sprite icons for file tabs, monochrome icons for tools,
 * black background, dark brown (#8a5027) top indicator, min/max width constraints, and text truncation (...).
 */
export const WorkspaceTabItem = React.memo(function WorkspaceTabItem({
  paneId,
  config,
  isActive,
  onSelect,
  onClose,
}: WorkspaceTabItemProps) {
  const title = getTabTitle(config);

  const handleDragStart = (e: React.DragEvent) => {
    useWorkspaceStore.getState().setDraggedTab({ sourcePaneId: paneId, tabConfig: config });
    const payload = JSON.stringify({ sourcePaneId: paneId, tabConfig: config });
    e.dataTransfer.setData("application/json", payload);
    e.dataTransfer.setData("text/plain", payload);
    e.dataTransfer.effectAllowed = "move";
  };

  const handleDragEnd = () => {
    useWorkspaceStore.getState().setDraggedTab(null);
  };

  const handleClose = (e: React.MouseEvent) => {
    e.stopPropagation();
    onClose();
  };

  return (
    <div
      draggable
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onClick={onSelect}
      style={{ WebkitUserDrag: "element" } as React.CSSProperties}
      className={`group relative flex items-center gap-2 px-3 py-1.5 h-9 min-w-[120px] max-w-[180px] rounded-none text-xs cursor-grab active:cursor-grabbing border-r border-border transition-colors select-none ${
        isActive
          ? "bg-black text-foreground font-medium border-t-2 border-t-[#8a5027]"
          : "bg-transparent text-foreground-muted hover:bg-white/[0.04] hover:text-foreground"
      }`}
      title={title}
    >
      {getTabIcon(config)}
      <span className="truncate flex-1 min-w-0">{title}</span>
      <button
        onClick={handleClose}
        className="w-4 h-4 shrink-0 flex items-center justify-center rounded-none text-foreground-muted hover:text-danger hover:bg-white/10 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer p-0"
        title="Close tab (⌘W)"
        aria-label={`Close ${title}`}
      >
        <X size={12} />
      </button>
    </div>
  );
});
