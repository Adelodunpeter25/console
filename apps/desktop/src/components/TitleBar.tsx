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
 * Custom draggable titlebar for Electron macOS overlay window.
 *
 * Uses `titleBarStyle: "hiddenInset"` with traffic lights at { x: 16, y: 14 }.
 * Draggable region uses `-webkit-app-region: drag` and buttons use `-webkit-app-region: no-drag`.
 */
export function TitleBar({ rightAction, title }: TitleBarProps) {
  const sidebarOpen = useAppStore((state) => state.sidebarOpen);
  const toggleSidebar = useAppStore((state) => state.toggleSidebar);
  const rightSidebarOpen = useAppStore((state) => state.rightSidebarOpen);
  const toggleRightSidebar = useAppStore((state) => state.toggleRightSidebar);

  return (
    <div
      style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
      className="flex items-center h-10 bg-sidebar border-b border-border shrink-0 select-none"
    >
      {/* Left: sidebar toggle (padded past traffic lights at x:16 on macOS) */}
      <div className="flex items-center pl-20 pr-2">
        <button
          style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
          onClick={(e) => {
            e.stopPropagation();
            toggleSidebar();
          }}
          className="p-1.5 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors cursor-pointer"
          title={sidebarOpen ? "Collapse left sidebar" : "Expand left sidebar"}
        >
          <HugeiconsIcon icon={SidebarLeftIcon} size={16} />
        </button>
      </div>

      {/* Center: optional title */}
      {title && (
        <div className="flex-1 text-center">
          <span className="text-xs text-foreground-muted font-medium">{title}</span>
        </div>
      )}
      {!title && <div className="flex-1" />}

      {/* Right: custom action + right sidebar toggle */}
      <div className="flex items-center gap-1 pr-2">
        {rightAction && (
          <button
            style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
            onClick={(e) => {
              e.stopPropagation();
              rightAction.onClick();
            }}
            className="flex items-center justify-center w-9 h-8 text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors cursor-pointer"
            title={rightAction.label}
          >
            {rightAction.icon}
          </button>
        )}
        <button
          style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
          onClick={(e) => {
            e.stopPropagation();
            toggleRightSidebar();
          }}
          className="p-1.5 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors cursor-pointer"
          title={rightSidebarOpen ? "Collapse right sidebar" : "Expand right sidebar"}
        >
          <HugeiconsIcon icon={SidebarRightIcon} size={16} />
        </button>
      </div>
    </div>
  );
}
