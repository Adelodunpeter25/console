import React from "react";
import { Minus, Square, X, Copy } from "lucide-react";

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
 * Check whether we're running inside a Tauri webview. The Tauri runtime
 * injects `__TAURI_INTERNALS__` onto the global window object. When running
 * in a plain browser (dev preview, screenshots), the window API is not
 * available and we gracefully skip native window controls.
 */
function isTauri(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

async function getCurrentTauriWindow() {
  if (!isTauri()) return null;
  const { getCurrentWindow } = await import("@tauri-apps/api/window");
  return getCurrentWindow();
}

/**
 * Custom frameless titlebar.
 *
 * The entire bar is a Tauri drag region (`data-tauri-drag-region`). Interactive
 * elements (buttons) stop propagation so they don't trigger window dragging.
 * Window controls (minimize / maximize / close) call the Tauri window API.
 * In browser preview mode (non-Tauri), the controls are hidden.
 */
export function TitleBar({ rightAction, title }: TitleBarProps) {
  const [maximized, setMaximized] = React.useState(false);
  const [tauriReady, setTauriReady] = React.useState(false);

  React.useEffect(() => {
    let unlistenFn: (() => void) | null = null;
    let active = true;

    getCurrentTauriWindow().then((win) => {
      if (!win || !active) return;
      setTauriReady(true);
      win.isMaximized().then(setMaximized).catch(() => {});
      win.onResized(() => {
        win.isMaximized().then(setMaximized).catch(() => {});
      }).then((unlisten) => {
        unlistenFn = unlisten;
      });
    }).catch(() => {});

    return () => {
      active = false;
      if (unlistenFn) unlistenFn();
    };
  }, []);

  const handleMinimize = () => getCurrentTauriWindow().then((w) => w?.minimize());
  const handleToggleMax = () => getCurrentTauriWindow().then((w) => w?.toggleMaximize());
  const handleClose = () => getCurrentTauriWindow().then((w) => w?.close());

  return (
    <div
      data-tauri-drag-region
      className="flex items-center h-10 bg-sidebar border-b border-border shrink-0 select-none"
    >
      {/* Left: app brand */}
      <div className="flex items-center gap-2 px-4" data-tauri-drag-region>
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

      {/* Right: custom action + window controls */}
      <div className="flex items-center">
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

        {tauriReady && (
          <>
            <button
              onClick={(e) => {
                e.stopPropagation();
                handleMinimize();
              }}
              className="flex items-center justify-center w-11 h-8 text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
              title="Minimize"
            >
              <Minus size={15} />
            </button>

            <button
              onClick={(e) => {
                e.stopPropagation();
                handleToggleMax();
              }}
              className="flex items-center justify-center w-11 h-8 text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
              title={maximized ? "Restore" : "Maximize"}
            >
              {maximized ? <Copy size={13} /> : <Square size={12} />}
            </button>

            <button
              onClick={(e) => {
                e.stopPropagation();
                handleClose();
              }}
              className="flex items-center justify-center w-11 h-8 text-foreground-secondary hover:text-white hover:bg-danger transition-colors"
              title="Close"
            >
              <X size={15} />
            </button>
          </>
        )}
      </div>
    </div>
  );
}
