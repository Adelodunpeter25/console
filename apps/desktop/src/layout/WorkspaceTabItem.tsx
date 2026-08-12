import React from "react";
import { MessageSquare, FileCode, Terminal, GitCompare, X } from "lucide-react";
import { WorkspaceTabConfig, getTabId, getTabTitle } from "./types";

interface WorkspaceTabItemProps {
  config: WorkspaceTabConfig;
  isActive: boolean;
  onSelect: () => void;
  onClose: () => void;
}

function getTabIcon(type: WorkspaceTabConfig["type"]) {
  switch (type) {
    case "chat":
      return <MessageSquare size={13} className="text-blue-400 shrink-0" />;
    case "file":
      return <FileCode size={13} className="text-amber-400 shrink-0" />;
    case "terminal":
      return <Terminal size={13} className="text-green-400 shrink-0" />;
    case "diff":
      return <GitCompare size={13} className="text-purple-400 shrink-0" />;
  }
}

/**
 * WorkspaceTabItem — Tab pill component with warm brown (#a96842) top active indicator.
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
      className={`group relative flex items-center gap-2 px-3 py-1.5 h-9 max-w-[200px] min-w-[120px] rounded-t-sm text-xs cursor-pointer border-r border-border transition-colors select-none ${
        isActive
          ? "bg-[#211d1a] text-foreground font-medium border-t-2 border-t-[#a96842]"
          : "bg-transparent text-foreground-muted hover:bg-white/[0.04] hover:text-foreground"
      }`}
      title={title}
    >
      {getTabIcon(config.type)}
      <span className="truncate flex-1 min-w-0">{title}</span>
      <button
        onClick={handleClose}
        className="w-4 h-4 shrink-0 flex items-center justify-center rounded text-foreground-muted hover:text-danger hover:bg-white/10 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer p-0"
        title="Close tab"
        aria-label={`Close ${title}`}
      >
        <X size={12} />
      </button>
    </div>
  );
});
