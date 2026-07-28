import React from "react";
import { PanelLeft, PanelLeftClose } from "lucide-react";
import { useAppStore } from "../store";

interface TitleBarProps {
  /** Right-side action button (e.g. settings gear or back-to-app). */
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
  const { sidebarOpen, toggleSidebar } = useAppStore();

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
          title={sidebarOpen ? "Collapse sidebar" : "Expand sidebar"}
        >
          {sidebarOpen ? <PanelLeftClose size={16} /> : <PanelLeft size={16} />}
        </button>
      </div>

      {/* Center: optional title */}
      {title && (
        <div className="flex-1 text-center" data-tauri-drag-region>
          <span className="text-xs text-foreground-muted font-medium">{title}</span>
        </div>
      )}
      {!title && <div className="flex-1" data-tauri-drag-region />}

      {/* Right: custom action only (window controls are native) */}
      {rightAction && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            rightAction.onClick();
          }}
          className="flex items-center justify-center w-9 h-8 text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors mr-1"
          title={rightAction.label}
        >
          {rightAction.icon}
        </button>
      )}
    </div>
  );
}
