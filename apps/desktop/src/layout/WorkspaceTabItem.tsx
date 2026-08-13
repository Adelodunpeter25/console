import React from "react";
import { X } from "lucide-react";
import { HugeiconsIcon } from "@hugeicons/react";
import { ChatIcon, ComputerTerminal01Icon } from "@hugeicons/core-free-icons";
import { WorkspaceTabConfig, getTabTitle } from "./types";
import { FileIcon } from "../components/file/FileIcon";
import { useWorkspaceStore } from "./useWorkspaceStore";

interface WorkspaceTabItemProps {
  paneId?: string;
  config: WorkspaceTabConfig;
  isActive: boolean;
  isFocused?: boolean;
  onSelect: () => void;
  onClose: () => void;
}

function getTabIcon(config: WorkspaceTabConfig, isActive: boolean) {
  const iconClass = isActive
    ? "text-foreground shrink-0"
    : "text-foreground-secondary group-hover:text-foreground transition-colors shrink-0";

  switch (config.type) {
    case "chat":
      return <HugeiconsIcon icon={ChatIcon} size={15} className={iconClass} />;
    case "terminal":
      return <HugeiconsIcon icon={ComputerTerminal01Icon} size={15} className={iconClass} />;
    case "file":
    case "diff":
      return <FileIcon fileName={config.path} className="w-[15px] h-[15px] shrink-0" />;
  }
}

/**
 * WorkspaceTabItem — Draggable tab pill.
 * Uses 15px high-contrast Hugeicons for Chat and Terminal tabs, Sprite FileIcons for source files,
 * and scoped active brown indicator (#8a5027) for the focused pane.
 */
export const WorkspaceTabItem = React.memo(function WorkspaceTabItem({
  paneId,
  config,
  isActive,
  isFocused = true,
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

  const activeStyles =
    isActive && isFocused
      ? "bg-black text-foreground font-medium border-t-2 border-t-[#8a5027]"
      : isActive
        ? "bg-black/50 text-foreground-secondary border-t-2 border-t-transparent"
        : "bg-transparent text-foreground-muted hover:bg-white/[0.04] hover:text-foreground border-t-2 border-t-transparent";

  return (
    <div
      draggable
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onClick={onSelect}
      style={{ WebkitUserDrag: "element" } as React.CSSProperties}
      className={`group relative flex items-center gap-2 px-3 py-1.5 h-9 min-w-[120px] max-w-[180px] rounded-none text-xs cursor-grab active:cursor-grabbing border-r border-border transition-colors select-none ${activeStyles}`}
      title={title}
    >
      {getTabIcon(config, isActive)}
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
