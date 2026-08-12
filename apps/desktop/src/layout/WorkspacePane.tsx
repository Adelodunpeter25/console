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

  const activeTabConfig = React.useMemo(() => {
    if (!node.activeTabId) return null;
    return node.tabs.find((t) => getTabId(t) === node.activeTabId) ?? null;
  }, [node.tabs, node.activeTabId]);

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    const rect = e.currentTarget.getBoundingClientRect();
    const pos = calcDropPosition(rect, e.clientX, e.clientY);
    setDropPos(pos);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    if (e.currentTarget.contains(e.relatedTarget as Node)) return;
    setDropPos(null);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    const pos = dropPos ?? "center";
    setDropPos(null);

    const json = e.dataTransfer.getData("application/json");
    if (!json) return;

    try {
      const data = JSON.parse(json);
      if (data.tabConfig) {
        dropTabOnPane(node.id, pos, data.tabConfig, data.sourcePaneId);
      }
    } catch {}
  };

  return (
    <div
      onClick={() => setActivePane(node.id)}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className="flex flex-col h-full w-full overflow-hidden bg-black relative"
    >
      <WorkspaceDropzone position={dropPos} />
      <WorkspaceTabBar pane={node} canClosePane={canClosePane} />
      <div className="flex-1 overflow-hidden relative bg-black">
        <WorkspaceContent config={activeTabConfig} />
      </div>
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
