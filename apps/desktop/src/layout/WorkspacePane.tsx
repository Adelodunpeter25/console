import React from "react";
import { Group, Panel, Separator } from "react-resizable-panels";
import { WorkspaceNode, LeafPaneNode, SplitPaneNode, getTabId } from "./types";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { WorkspaceTabBar } from "./WorkspaceTabBar";
import { WorkspaceContent } from "./WorkspaceContent";
import { WorkspaceDropzone, calcDropPosition, DropPosition } from "./WorkspaceDropzone";

interface WorkspacePaneProps {
  node: WorkspaceNode;
  canClosePane?: boolean;
}

function SplitPaneView({ node }: { node: SplitPaneNode }) {
  const isHorizontal = node.direction === "horizontal";
  return (
    <Group orientation={node.direction} className="w-full h-full">
      <Panel defaultSize={node.sizes[0]} minSize={15}>
        <WorkspacePane node={node.children[0]} canClosePane={true} />
      </Panel>

      <Separator
        className={
          isHorizontal
            ? "w-[1px] h-full bg-border hover:bg-[#8a5027] transition-colors cursor-col-resize shrink-0"
            : "h-[1px] w-full bg-border hover:bg-[#8a5027] transition-colors cursor-row-resize shrink-0"
        }
      />

      <Panel defaultSize={node.sizes[1]} minSize={15}>
        <WorkspacePane node={node.children[1]} canClosePane={true} />
      </Panel>
    </Group>
  );
}

function LeafPaneView({ node, canClosePane }: { node: LeafPaneNode; canClosePane: boolean }) {
  const setActivePane = useWorkspaceStore((state) => state.setActivePane);
  const dropTabOnPane = useWorkspaceStore((state) => state.dropTabOnPane);
  const [dropPos, setDropPos] = React.useState<DropPosition | null>(null);
  const paneRef = React.useRef<HTMLDivElement>(null);

  const activeTabConfig = React.useMemo(() => {
    if (!node.activeTabId) return null;
    return node.tabs.find((t) => getTabId(t) === node.activeTabId) ?? null;
  }, [node.tabs, node.activeTabId]);

  const clearDropPos = React.useCallback(() => {
    setDropPos(null);
  }, []);

  // Clear highlight if the drag ends anywhere (cancel, drop outside, etc.)
  React.useEffect(() => {
    window.addEventListener("dragend", clearDropPos);
    return () => window.removeEventListener("dragend", clearDropPos);
  }, [clearDropPos]);

  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (rect.width > 0 && rect.height > 0) {
      const pos = calcDropPosition(rect, e.clientX, e.clientY);
      setDropPos(pos);
    }
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = "move";

    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return;

    const pos = calcDropPosition(rect, e.clientX, e.clientY);
    setDropPos((prev) => (prev === pos ? prev : pos));
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();

    // Only clear when the pointer actually leaves the pane
    const next = e.relatedTarget as Node | null;
    if (next && e.currentTarget.contains(next)) return;

    if (!next) {
      const rect = e.currentTarget.getBoundingClientRect();
      const { clientX, clientY } = e;
      if (
        clientX >= rect.left &&
        clientX <= rect.right &&
        clientY >= rect.top &&
        clientY <= rect.bottom
      ) {
        return;
      }
    }

    setDropPos(null);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();

    const rect = e.currentTarget.getBoundingClientRect();
    const finalPos = dropPos ?? calcDropPosition(rect, e.clientX, e.clientY);
    setDropPos(null);

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
      dropTabOnPane(node.id, finalPos, tabConfig, sourcePaneId);
    }
    useWorkspaceStore.getState().setDraggedTab(null);
  };

  return (
    <div
      ref={paneRef}
      onClick={() => setActivePane(node.id)}
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className="flex flex-col h-full w-full overflow-hidden bg-black relative isolate"
    >
      <WorkspaceTabBar pane={node} canClosePane={canClosePane} />
      <div className="flex-1 overflow-hidden relative bg-black min-h-0">
        <WorkspaceContent config={activeTabConfig} />
      </div>
      {/* Render last so the overlay always paints above tab bar + content */}
      <WorkspaceDropzone position={dropPos} />
    </div>
  );
}

/**
 * WorkspacePane — Recursive renderer for cmux-style split tiles and leaf panes.
 */
export function WorkspacePane({ node, canClosePane = false }: WorkspacePaneProps) {
  if (node.type === "split") {
    return <SplitPaneView node={node} />;
  }
  return <LeafPaneView node={node} canClosePane={canClosePane} />;
}
