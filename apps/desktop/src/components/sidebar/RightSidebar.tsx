import React from "react";
import { FolderTree, Plus, RefreshCw, X } from "lucide-react";
import { HugeiconsIcon } from "@hugeicons/react";
import { ComputerTerminal01Icon } from "@hugeicons/core-free-icons";
import { Group, Panel, Separator } from "react-resizable-panels";
import { MAX_DOCKED_TERMINALS, useAppStore } from "../../store/useAppStore";
import { useProjectStore } from "../../store/useProjectStore";
import { useFsStore } from "../../store/useFsStore";
import { FileTree } from "../file/FileTree";
import { useProjectFsWatcher } from "../../hooks/useProjectFsWatcher";
import { useWorkspaceStore } from "../../layout/useWorkspaceStore";
import { useTerminalStore } from "../../store/useTerminalStore";
import { TerminalTab } from "../terminal/TerminalTab";
import { getTabId } from "../../layout/types";

export function RightSidebar({ width = 288 }: { width?: number }) {
  const selectedProjectId = useAppStore((state) => state.selectedProjectId);
  const rightSidebarPanelSizes = useAppStore((state) => state.rightSidebarPanelSizes);
  const setRightSidebarPanelSizes = useAppStore((state) => state.setRightSidebarPanelSizes);
  const projects = useProjectStore((state) => state.projects);
  const browse = useFsStore((state) => state.browse);
  const browseDirectory = useFsStore((state) => state.browseDirectory);
  const openFileTab = useWorkspaceStore((state) => state.openFileTab);
  const dockedTerminals = useAppStore((state) => state.dockedTerminals);
  const activeDockedTerminalId = useAppStore((state) => state.activeDockedTerminalId);
  const dockTerminal = useAppStore((state) => state.dockTerminal);
  const setActiveDockedTerminal = useAppStore((state) => state.setActiveDockedTerminal);
  const removeDockedTerminal = useAppStore((state) => state.removeDockedTerminal);
  const openTerminal = useTerminalStore((state) => state.openTerminal);
  const killTerminal = useTerminalStore((state) => state.kill);

  const [isOverBottom, setIsOverBottom] = React.useState(false);
  const asideRef = React.useRef<HTMLElement>(null);

  const currentProject = React.useMemo(
    () => projects.find((p) => p.id === selectedProjectId),
    [projects, selectedProjectId],
  );

  const projectPath = currentProject?.path;

  useProjectFsWatcher(projectPath);

  const tree = React.useMemo(() => {
    if (!browse || !projectPath) return [];
    const normBrowse = browse.path.replace(/\/$/, "");
    const normProject = projectPath.replace(/\/$/, "");
    return normBrowse === normProject ? browse.entries : [];
  }, [browse, projectPath]);

  const handleRefresh = () => {
    if (projectPath) {
      browseDirectory(projectPath).catch(() => {});
    }
  };

  const handleFileSelect = React.useCallback(
    (filePath: string) => {
      if (!selectedProjectId) return;
      openFileTab({ type: "file", projectId: selectedProjectId, path: filePath });
    },
    [openFileTab, selectedProjectId],
  );

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = "move";

    if (!asideRef.current) return;
    const rect = asideRef.current.getBoundingClientRect();
    if (rect.height <= 0) return;

    const yRatio = (e.clientY - rect.top) / rect.height;
    setIsOverBottom(yRatio > 0.35);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const next = e.relatedTarget as Node | null;
    if (next && asideRef.current?.contains(next)) return;
    setIsOverBottom(false);
  };

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsOverBottom(false);

    const dragged = useWorkspaceStore.getState().draggedTab;
    let tabConfig = dragged?.tabConfig;
    let sourcePaneId = dragged?.sourcePaneId;

    if (!tabConfig) {
      const json = e.dataTransfer.getData("application/json") || e.dataTransfer.getData("text/plain");
      if (json) {
        try {
          const data = JSON.parse(json);
          tabConfig = data.tabConfig;
          sourcePaneId = data.sourcePaneId;
        } catch {
          // ignore malformed payloads
        }
      }
    }

    if (tabConfig) {
      const isExistingDockedTerminal =
        tabConfig.type === "terminal" &&
        dockedTerminals.some((terminal) => terminal.terminalId === tabConfig.terminalId);
      if (dockedTerminals.length >= MAX_DOCKED_TERMINALS && !isExistingDockedTerminal) {
        useWorkspaceStore.getState().setDraggedTab(null);
        return;
      }

      if (sourcePaneId) {
        const workspace = useWorkspaceStore.getState();
        if (tabConfig.type === "terminal") {
          workspace.detachTab(sourcePaneId, getTabId(tabConfig));
        } else {
          workspace.closeTab(sourcePaneId, getTabId(tabConfig));
        }
      }
      if (tabConfig.type === "terminal") {
        dockTerminal(tabConfig);
      } else {
        // If a non-terminal was dropped at the bottom, spawn a terminal for that project
        const pId = tabConfig.projectId || selectedProjectId;
        const pPath = projects.find((p) => p.id === pId)?.path;
        if (pId && pPath) {
          try {
            const terminal = await openTerminal({ projectId: pId, cwd: pPath });
            const accepted = dockTerminal({
              type: "terminal",
              projectId: pId,
              terminalId: terminal.id,
              title: "Terminal",
            });
            if (!accepted) void killTerminal(terminal.id);
          } catch {}
        }
      }
    }
    useWorkspaceStore.getState().setDraggedTab(null);
  };

  const activeDockedTerminal = dockedTerminals.find(
    (terminal) => terminal.terminalId === activeDockedTerminalId,
  ) ?? dockedTerminals[0];

  const handleNewTerminal = async () => {
    if (dockedTerminals.length >= MAX_DOCKED_TERMINALS) return;

    const projectId = selectedProjectId ?? activeDockedTerminal?.projectId;
    const path = projects.find((project) => project.id === projectId)?.path;
    if (!projectId || !path) return;

    try {
      const terminal = await openTerminal({ projectId, cwd: path });
      const accepted = dockTerminal({
        type: "terminal",
        projectId,
        terminalId: terminal.id,
        title: "Terminal",
      });
      if (!accepted) void killTerminal(terminal.id);
    } catch {
      // The terminal store owns the error state for failed spawns.
    }
  };

  const handleCloseTerminal = (terminalId: string) => {
    removeDockedTerminal(terminalId);
    void killTerminal(terminalId);
  };

  const explorerContent = (
    <div className="flex flex-col h-full overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 border-b border-border shrink-0">
        <div className="flex items-center gap-2 text-foreground font-medium text-xs">
          <FolderTree size={14} className="text-primary-light" />
          <span className="truncate">{currentProject?.name ?? "Explorer"}</span>
        </div>
        {projectPath && (
          <button
            onClick={handleRefresh}
            title="Refresh File Tree"
            className="p-1 text-foreground-muted hover:text-foreground hover:bg-surface-hover rounded transition-colors"
          >
            <RefreshCw size={12} />
          </button>
        )}
      </div>

      <div className="flex-1 overflow-y-auto min-h-0">
        {projectPath ? (
          <FileTree tree={tree} onFileSelect={handleFileSelect} />
        ) : (
          <div className="p-4 text-xs text-foreground-muted text-center">
            No active project selected.
          </div>
        )}
      </div>
    </div>
  );

  return (
    <aside
      ref={asideRef}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      style={{ width }}
      className="relative flex flex-col h-full bg-sidebar border-l border-border select-none overflow-hidden shrink-0"
    >
      {dockedTerminals.length > 0 ? (
        <Group
          orientation="vertical"
          className="h-full w-full"
          onLayoutChanged={(layout) => {
            const explorerSize = layout["right-sidebar-explorer"];
            const terminalSize = layout["right-sidebar-terminal"];
            if (explorerSize !== undefined && terminalSize !== undefined) {
              setRightSidebarPanelSizes([explorerSize, terminalSize]);
            }
          }}
        >
          <Panel id="right-sidebar-explorer" defaultSize={rightSidebarPanelSizes[0]} minSize={20}>
            {explorerContent}
          </Panel>

          <Separator className="h-[1px] w-full bg-border hover:bg-[#8a5027] transition-colors cursor-row-resize shrink-0" />

          <Panel id="right-sidebar-terminal" defaultSize={rightSidebarPanelSizes[1]} minSize={20}>
            <div className="flex flex-col h-full bg-[#0d0d0d] overflow-hidden">
              <div className="flex items-center h-8 bg-sidebar border-b border-border text-xs text-foreground shrink-0">
                <div className="flex items-center h-full min-w-0 flex-1 overflow-x-auto no-scrollbar">
                  {dockedTerminals.map((terminal) => {
                    const isActive = terminal.terminalId === activeDockedTerminal?.terminalId;
                    return (
                      <div
                        key={terminal.terminalId}
                        className={`group flex items-center h-full min-w-0 max-w-[150px] border-r border-border ${
                          isActive ? "bg-[#0d0d0d]" : "bg-sidebar hover:bg-surface-hover"
                        }`}
                      >
                        <button
                          onClick={() => setActiveDockedTerminal(terminal.terminalId)}
                          title={terminal.title ?? "Terminal"}
                          className={`flex items-center gap-1.5 min-w-0 px-2.5 h-full text-left cursor-pointer ${
                            isActive ? "text-foreground" : "text-foreground-muted"
                          }`}
                        >
                          <HugeiconsIcon
                            icon={ComputerTerminal01Icon}
                            size={13}
                            className="shrink-0"
                          />
                          <span className="truncate">{terminal.title ?? "Terminal"}</span>
                        </button>
                        <button
                          onClick={() => handleCloseTerminal(terminal.terminalId)}
                          title={`Close ${terminal.title ?? "Terminal"}`}
                          aria-label={`Close ${terminal.title ?? "Terminal"}`}
                          className="mr-1 p-1 text-foreground-muted hover:text-danger rounded hover:bg-white/10 transition-colors cursor-pointer opacity-70 group-hover:opacity-100"
                        >
                          <X size={11} />
                        </button>
                      </div>
                    );
                  })}
                </div>
                <button
                  onClick={() => void handleNewTerminal()}
                  disabled={dockedTerminals.length >= MAX_DOCKED_TERMINALS}
                  title={
                    dockedTerminals.length >= MAX_DOCKED_TERMINALS
                      ? "Maximum of 3 terminals"
                      : "New Terminal"
                  }
                  aria-label="New Terminal"
                  className="p-1.5 mr-1 text-foreground-muted hover:text-foreground rounded hover:bg-white/10 transition-colors cursor-pointer shrink-0 disabled:opacity-40 disabled:cursor-not-allowed"
                >
                  <Plus size={13} />
                </button>
              </div>
              <div className="flex-1 overflow-hidden min-h-0 relative">
                {dockedTerminals.map((terminal) => (
                  <div
                    key={terminal.terminalId}
                    className={`absolute inset-0 ${
                      terminal.terminalId === activeDockedTerminal?.terminalId ? "" : "hidden"
                    }`}
                  >
                    <TerminalTab config={terminal} />
                  </div>
                ))}
              </div>
            </div>
          </Panel>
        </Group>
      ) : (
        explorerContent
      )}

      {/* Bottom Dropzone Highlight Overlay */}
      {isOverBottom && (
        <div className="absolute bottom-0 left-0 right-0 h-1/2 z-50 pointer-events-none bg-dropzone-bg border-t-2 border-t-dropzone-border backdrop-blur-[1px] flex items-center justify-center">
          <span className="px-2.5 py-1 bg-dropzone-badge-bg text-dropzone-badge-text text-[11px] font-mono rounded border border-dropzone-badge-border uppercase tracking-wide shadow-lg">
            Dock Terminal Bottom
          </span>
        </div>
      )}
    </aside>
  );
}
