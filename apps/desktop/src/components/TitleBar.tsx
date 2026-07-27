import React from "react";

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
  return (
    <div
      data-tauri-drag-region
      className="flex items-center h-10 bg-sidebar border-b border-border shrink-0 select-none"
    >
      {/* Left: app brand (padded past traffic lights on macOS) */}
      <div className="flex items-center gap-2 pl-20 px-4" data-tauri-drag-region>
        <div className="w-5 h-5 rounded bg-white/10 border border-border flex items-center justify-center">
          <span className="text-xs font-bold text-foreground">C</span>
        </div>
        <span className="text-sm font-semibold text-foreground tracking-tight">
          Console
        </span>
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
