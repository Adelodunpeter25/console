import React from "react";
import { TitleBar } from "../components/TitleBar";
import { Sidebar } from "../components/sidebar/Sidebar";
import { RightSidebar } from "../components/sidebar/RightSidebar";
import { WorkspaceLayout } from "../layout";
import { ResizablePanel } from "../components/common/ResizablePanel";
import { CommandPalette } from "../components/commandpalette/CommandPalette";
import { useAppStore } from "../store/useAppStore";
import { useProjectStore } from "../store/useProjectStore";
import { useServerStore } from "../store/useServerStore";
import { useWorkspaceStore } from "../layout/useWorkspaceStore";
import { findLeaf, findFirstLeaf } from "../layout/treeHelpers";
import { getTabId } from "../layout/types";
import { basename } from "../utils/format";
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
  const paletteOpen = useAppStore((state) => state.commandPaletteOpen);
  const setPaletteOpen = useAppStore((state) => state.setCommandPaletteOpen);
  const init = useServerStore((state) => state.init);

  const rootNode = useWorkspaceStore((state) => state.rootNode);
  const activePaneId = useWorkspaceStore((state) => state.activePaneId);
  const projects = useProjectStore((state) => state.projects);

  const [sidebarWidth, setSidebarWidthState] = React.useState(SIDEBAR_DEFAULT);
  const [rightSidebarWidth, setRightSidebarWidthState] = React.useState(SIDEBAR_DEFAULT);

  // Derive active workspace title: "Chat Title — Project"
  const activeTitle = React.useMemo(() => {
    const activeLeaf = findLeaf(rootNode, activePaneId) ?? findFirstLeaf(rootNode);
    if (!activeLeaf || !activeLeaf.activeTabId) return "Console";

    const activeTab = activeLeaf.tabs.find((t) => getTabId(t) === activeLeaf.activeTabId);
    if (!activeTab) return "Console";

    const tabTitle =
      activeTab.title ||
      (activeTab.type === "chat"
        ? "Chat"
        : activeTab.type === "terminal"
          ? "Terminal"
          : "File");

    const project = projects.find((p) => p.id === activeTab.projectId);
    const projectName = project?.path ? basename(project.path) : "";

    return projectName ? `${tabTitle} — ${projectName}` : tabTitle;
  }, [rootNode, activePaneId, projects]);

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
        setPaletteOpen(!useAppStore.getState().commandPaletteOpen);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [setPaletteOpen]);

  return (
    <div className="flex flex-col h-screen w-screen bg-screen overflow-hidden">
      <TitleBar title={activeTitle} />
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
