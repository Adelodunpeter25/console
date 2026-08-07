import React from "react";
import { HugeiconsIcon } from "@hugeicons/react";
import { SidebarLeftIcon, SidebarRightIcon } from "@hugeicons/core-free-icons";
import { useAppStore } from "../store/useAppStore";

interface TitleBarProps {
  /** Right-side action button (e.g. back-to-app). */
  rightAction?: {
    icon: React.ReactNode;
    label: string;
    onClick: () => void;
  };
  /** Optional center title shown in the drag region. */
  title?: string;
}

/**
 * Custom titlebar for native overlay window.
 *
 * Uses `titleBarStyle: "Overlay"` so macOS traffic lights (and Linux/Windows
 * native controls) are rendered by the OS on top of this bar. The bar itself
 * is a Tauri drag region (`data-tauri-drag-region`) with left padding to
 * accommodate the traffic lights at {x:14, y:20}.
 *
 * No custom window control buttons — the OS provides minimize/maximize/close.
 */
export function TitleBar({ rightAction, title }: TitleBarProps) {
  const sidebarOpen = useAppStore((state) => state.sidebarOpen);
  const toggleSidebar = useAppStore((state) => state.toggleSidebar);
  const rightSidebarOpen = useAppStore((state) => state.rightSidebarOpen);
  const toggleRightSidebar = useAppStore((state) => state.toggleRightSidebar);

  return (
    <div
      data-tauri-drag-region
      className="flex items-center h-10 bg-sidebar border-b border-border shrink-0 select-none"
    >
      {/* Left: sidebar toggle (padded past traffic lights on macOS) */}
      <div className="flex items-center pl-20 px-2" data-tauri-drag-region>
        <button
          onClick={(e) => {
            e.stopPropagation();
            toggleSidebar();
          }}
          className="p-1.5 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
          title={sidebarOpen ? "Collapse left sidebar" : "Expand left sidebar"}
        >
          <HugeiconsIcon icon={SidebarLeftIcon} size={16} />
        </button>
      </div>

      {/* Center: optional title */}
      {title && (
        <div className="flex-1 text-center" data-tauri-drag-region>
          <span className="text-xs text-foreground-muted font-medium">{title}</span>
        </div>
      )}
      {!title && <div className="flex-1" data-tauri-drag-region />}

      {/* Right: custom action + right sidebar toggle */}
      <div className="flex items-center gap-1 pr-2" data-tauri-drag-region>
        {rightAction && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              rightAction.onClick();
            }}
            className="flex items-center justify-center w-9 h-8 text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
            title={rightAction.label}
          >
            {rightAction.icon}
          </button>
        )}
        <button
          onClick={(e) => {
            e.stopPropagation();
            toggleRightSidebar();
          }}
          className="p-1.5 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
          title={rightSidebarOpen ? "Collapse right sidebar" : "Expand right sidebar"}
        >
          <HugeiconsIcon icon={SidebarRightIcon} size={16} />
        </button>
      </div>
    </div>
  );
}
