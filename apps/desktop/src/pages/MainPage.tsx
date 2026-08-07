import React from "react";
import { TitleBar } from "../components/TitleBar";
import { Sidebar } from "../components/sidebar/Sidebar";
import { RightSidebar } from "../components/sidebar/RightSidebar";
import { WorkspaceLayout } from "../layout";
import { ResizablePanel } from "../components/common/ResizablePanel";
import { CommandPalette } from "../components/commandpalette/CommandPalette";
import { useAppStore } from "../store/useAppStore";
import { useServerStore } from "../store/useServerStore";
import {
  getSidebarWidth,
  setSidebarWidth,
  getRightSidebarWidth,
  setRightSidebarWidth,
  getSidebarOpen,
  getRightSidebarOpen,
} from "../lib/ui-store";

const SIDEBAR_MIN = 200;
const SIDEBAR_MAX = 480;
const SIDEBAR_DEFAULT = 288;

/**
 * MainPage — Top-level shell layout container.
 *
 * Owns the outer application layout frame: TitleBar, Left Sidebar, Center Workspace Dock, and Right Sidebar.
 */
export function MainPage() {
  const sidebarOpen = useAppStore((state) => state.sidebarOpen);
  const rightSidebarOpen = useAppStore((state) => state.rightSidebarOpen);
  const setSidebarOpen = useAppStore((state) => state.setSidebarOpen);
  const setRightSidebarOpen = useAppStore((state) => state.setRightSidebarOpen);
  const { init } = useServerStore();
  const [paletteOpen, setPaletteOpen] = React.useState(false);
  const [sidebarWidth, setSidebarWidthState] = React.useState(SIDEBAR_DEFAULT);
  const [rightSidebarWidth, setRightSidebarWidthState] = React.useState(SIDEBAR_DEFAULT);

  // Restore persisted sidebar widths and visibility on mount.
  React.useEffect(() => {
    init();
    getSidebarWidth().then((w) => {
      if (w != null) setSidebarWidthState(Math.min(Math.max(w, SIDEBAR_MIN), SIDEBAR_MAX));
    });
    getRightSidebarWidth().then((w) => {
      if (w != null) setRightSidebarWidthState(Math.min(Math.max(w, SIDEBAR_MIN), SIDEBAR_MAX));
    });
    getSidebarOpen().then((open) => {
      if (open != null) setSidebarOpen(open);
    });
    getRightSidebarOpen().then((open) => {
      if (open != null) setRightSidebarOpen(open);
    });
  }, [init, setSidebarOpen, setRightSidebarOpen]);

  const handleSidebarResizeEnd = (width: number) => {
    setSidebarWidth(width).catch(() => {});
  };

  const handleRightSidebarResizeEnd = (width: number) => {
    setRightSidebarWidth(width).catch(() => {});
  };

  // ⌘K / Ctrl+K toggles the command palette
  React.useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  return (
    <div className="flex flex-col h-screen w-screen bg-screen overflow-hidden">
      <TitleBar />
      <div className="flex flex-1 overflow-hidden">
        {sidebarOpen && (
          <ResizablePanel
            width={sidebarWidth}
            onWidthChange={setSidebarWidthState}
            minWidth={SIDEBAR_MIN}
            maxWidth={SIDEBAR_MAX}
            onResizeEnd={handleSidebarResizeEnd}
            handleSide="right"
          >
            <Sidebar width={sidebarWidth} />
          </ResizablePanel>
        )}
        <div className="flex-1 flex flex-col h-full overflow-hidden">
          <WorkspaceLayout />
        </div>
        {rightSidebarOpen && (
          <ResizablePanel
            width={rightSidebarWidth}
            onWidthChange={setRightSidebarWidthState}
            minWidth={SIDEBAR_MIN}
            maxWidth={SIDEBAR_MAX}
            onResizeEnd={handleRightSidebarResizeEnd}
            handleSide="left"
          >
            <RightSidebar width={rightSidebarWidth} />
          </ResizablePanel>
        )}
      </div>
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
    </div>
  );
}
