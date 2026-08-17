import React from "react";
import { TitleBar } from "../components/TitleBar";
import { Sidebar } from "../components/sidebar/Sidebar";
import { RightSidebar } from "../components/sidebar/RightSidebar";
import { WorkspaceLayout } from "../layout";
import { ResizablePanel } from "../components/common/ResizablePanel";
import { CommandPalette } from "../components/commandpalette/CommandPalette";
import { useAppStore } from "../store/useAppStore";
import { useProjectStore } from "../store/useProjectStore";
import { useWorkspaceStore } from "../layout/useWorkspaceStore";
import { findLeaf, findFirstLeaf } from "../layout/treeHelpers";
import { getTabId } from "../layout/types";
import { basename } from "../utils/format";

const SIDEBAR_MIN = 200;
const SIDEBAR_MAX = 480;

/**
 * MainPage — Top-level shell layout container.
 *
 * Owns the outer application layout frame: TitleBar, Left Sidebar, Center Workspace Dock, and Right Sidebar.
 */
export function MainPage() {
  const sidebarOpen = useAppStore((state) => state.sidebarOpen);
  const rightSidebarOpen = useAppStore((state) => state.rightSidebarOpen);
  const sidebarWidth = useAppStore((state) => state.sidebarWidth);
  const rightSidebarWidth = useAppStore((state) => state.rightSidebarWidth);
  const setSidebarWidth = useAppStore((state) => state.setSidebarWidth);
  const setRightSidebarWidth = useAppStore((state) => state.setRightSidebarWidth);
  const paletteOpen = useAppStore((state) => state.commandPaletteOpen);
  const setPaletteOpen = useAppStore((state) => state.setCommandPaletteOpen);

  const rootNode = useWorkspaceStore((state) => state.rootNode);
  const activePaneId = useWorkspaceStore((state) => state.activePaneId);
  const projects = useProjectStore((state) => state.projects);

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
            onWidthChange={setSidebarWidth}
            minWidth={SIDEBAR_MIN}
            maxWidth={SIDEBAR_MAX}
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
            onWidthChange={setRightSidebarWidth}
            minWidth={SIDEBAR_MIN}
            maxWidth={SIDEBAR_MAX}
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
