import React from "react";
import { MessageSquare, FileCode, Terminal, GitCompare, X } from "lucide-react";
import { WorkspaceTabConfig, getTabTitle } from "./types";

interface WorkspaceTabItemProps {
  config: WorkspaceTabConfig;
  isActive: boolean;
  onSelect: () => void;
  onClose: () => void;
}

function getTabIcon(type: WorkspaceTabConfig["type"]) {
  const iconClass = "text-foreground-muted shrink-0";
  switch (type) {
    case "chat":
      return <MessageSquare size={13} className={iconClass} />;
    case "file":
      return <FileCode size={13} className={iconClass} />;
    case "terminal":
      return <Terminal size={13} className={iconClass} />;
    case "diff":
      return <GitCompare size={13} className={iconClass} />;
  }
}

/**
 * WorkspaceTabItem — Boxy tab pill with monochrome icons, black background,
 * dark brown (#8a5027) top indicator, min/max width constraints, and text truncation (...).
 */
export const WorkspaceTabItem = React.memo(function WorkspaceTabItem({
  config,
  isActive,
  onSelect,
  onClose,
}: WorkspaceTabItemProps) {
  const title = getTabTitle(config);

  const handleClose = (e: React.MouseEvent) => {
    e.stopPropagation();
    onClose();
  };

  return (
    <div
      onClick={onSelect}
      className={`group relative flex items-center gap-2 px-3 py-1.5 h-9 min-w-[120px] max-w-[180px] rounded-none text-xs cursor-pointer border-r border-border transition-colors select-none ${
        isActive
          ? "bg-black text-foreground font-medium border-t-2 border-t-[#8a5027]"
          : "bg-transparent text-foreground-muted hover:bg-white/[0.04] hover:text-foreground"
      }`}
      title={title}
    >
      {getTabIcon(config.type)}
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
