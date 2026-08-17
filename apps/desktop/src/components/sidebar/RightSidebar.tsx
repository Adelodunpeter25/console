import React from "react";
import { FolderTree, RefreshCw, X } from "lucide-react";
import { HugeiconsIcon } from "@hugeicons/react";
import { ComputerTerminal01Icon } from "@hugeicons/core-free-icons";
import { Group, Panel, Separator } from "react-resizable-panels";
import { toast } from "sonner";
import { useAppStore } from "../../store/useAppStore";
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
  const projects = useProjectStore((state) => state.projects);
  const browse = useFsStore((state) => state.browse);
  const browseDirectory = useFsStore((state) => state.browseDirectory);
  const openFileTab = useWorkspaceStore((state) => state.openFileTab);
  const dockedTerminal = useAppStore((state) => state.dockedTerminal);
  const setDockedTerminal = useAppStore((state) => state.setDockedTerminal);
  const openTerminal = useTerminalStore((state) => state.openTerminal);

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

  const handleNewTerminal = async () => {
    if (!selectedProjectId || !projectPath) {
      toast.error("Select an active project first to open a terminal.");
      return;
    }

    try {
      const terminal = await openTerminal({ projectId: selectedProjectId, cwd: projectPath });
      setDockedTerminal({
        type: "terminal",
        projectId: selectedProjectId,
        terminalId: terminal.id,
        title: "Terminal",
      });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to open terminal.");
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
      if (sourcePaneId) {
        useWorkspaceStore.getState().closeTab(sourcePaneId, getTabId(tabConfig));
      }
      if (tabConfig.type === "terminal") {
        setDockedTerminal(tabConfig);
      } else {
        // If a non-terminal was dropped at the bottom, spawn a terminal for that project
        const pId = tabConfig.projectId || selectedProjectId;
        const pPath = projects.find((p) => p.id === pId)?.path;
        if (pId && pPath) {
          try {
            const terminal = await openTerminal({ projectId: pId, cwd: pPath });
            setDockedTerminal({
              type: "terminal",
              projectId: pId,
              terminalId: terminal.id,
              title: "Terminal",
            });
          } catch {}
        }
      }
    }
    useWorkspaceStore.getState().setDraggedTab(null);
  };

  const explorerContent = (
    <div className="flex flex-col h-full overflow-hidden">
      <div className="flex items-center justify-between px-3 py-2 border-b border-border shrink-0">
        <div className="flex items-center gap-2 text-foreground font-medium text-xs">
          <FolderTree size={14} className="text-primary-light" />
          <span className="truncate">{currentProject?.name ?? "Explorer"}</span>
        </div>
        {projectPath && (
          <div className="flex items-center gap-1">
            <button
              onClick={handleNewTerminal}
              title="New Terminal"
              className="p-1 text-foreground-muted hover:text-foreground hover:bg-surface-hover rounded transition-colors"
            >
              <HugeiconsIcon icon={ComputerTerminal01Icon} size={13} />
            </button>
            <button
              onClick={handleRefresh}
              title="Refresh File Tree"
              className="p-1 text-foreground-muted hover:text-foreground hover:bg-surface-hover rounded transition-colors"
            >
              <RefreshCw size={12} />
            </button>
          </div>
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
      {dockedTerminal ? (
        <Group orientation="vertical" className="h-full w-full">
          <Panel defaultSize={55} minSize={20}>
            {explorerContent}
          </Panel>

          <Separator className="h-[1px] w-full bg-border hover:bg-[#8a5027] transition-colors cursor-row-resize shrink-0" />

          <Panel defaultSize={45} minSize={20}>
            <div className="flex flex-col h-full bg-[#0d0d0d] overflow-hidden">
              <div className="flex items-center justify-between px-3 py-1.5 bg-sidebar border-b border-border text-xs text-foreground shrink-0">
                <div className="flex items-center gap-1.5 font-medium">
                  <HugeiconsIcon
                    icon={ComputerTerminal01Icon}
                    size={14}
                    className="text-foreground-secondary shrink-0"
                  />
                  <span className="truncate">{dockedTerminal.title ?? "Terminal"}</span>
                </div>
                <button
                  onClick={() => setDockedTerminal(null)}
                  title="Close / Undock Terminal"
                  className="p-1 text-foreground-muted hover:text-danger rounded hover:bg-white/10 transition-colors cursor-pointer"
                >
                  <X size={12} />
                </button>
              </div>
              <div className="flex-1 overflow-hidden min-h-0 relative">
                <TerminalTab config={dockedTerminal} />
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
