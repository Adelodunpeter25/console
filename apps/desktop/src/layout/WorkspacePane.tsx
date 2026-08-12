import React from "react";
import { Group, Panel, Separator } from "react-resizable-panels";
import { WorkspaceNode, getTabId } from "./types";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { WorkspaceTabBar } from "./WorkspaceTabBar";
import { WorkspaceContent } from "./WorkspaceContent";

interface WorkspacePaneProps {
  node: WorkspaceNode;
  canClosePane?: boolean;
}

/**
 * WorkspacePane — Recursive renderer for cmux-style split tiles and leaf panes.
 */
export function WorkspacePane({ node, canClosePane = false }: WorkspacePaneProps) {
  const activePaneId = useWorkspaceStore((state) => state.activePaneId);
  const setActivePane = useWorkspaceStore((state) => state.setActivePane);

  if (node.type === "split") {
    const isHorizontal = node.direction === "horizontal";
    return (
      <Group orientation={node.direction} className="w-full h-full">
        <Panel defaultSize={node.sizes[0]} minSize={15}>
          <WorkspacePane node={node.children[0]} canClosePane={true} />
        </Panel>

        <Separator
          className={
            isHorizontal
              ? "w-1 h-full bg-border hover:bg-amber-500/80 transition-colors cursor-col-resize shrink-0"
              : "h-1 w-full bg-border hover:bg-amber-500/80 transition-colors cursor-row-resize shrink-0"
          }
        />

        <Panel defaultSize={node.sizes[1]} minSize={15}>
          <WorkspacePane node={node.children[1]} canClosePane={true} />
        </Panel>
      </Group>
    );
  }

  // Leaf Pane
  const isActivePane = activePaneId === node.id;
  const activeTabConfig = React.useMemo(() => {
    if (!node.activeTabId) return null;
    return node.tabs.find((t) => getTabId(t) === node.activeTabId) ?? null;
  }, [node.tabs, node.activeTabId]);

  return (
    <div
      onClick={() => setActivePane(node.id)}
      className={`flex flex-col h-full w-full overflow-hidden bg-screen ${
        isActivePane ? "ring-1 ring-amber-500/30" : ""
      }`}
    >
      <WorkspaceTabBar pane={node} canClosePane={canClosePane} />
      <div className="flex-1 overflow-hidden relative">
        <WorkspaceContent config={activeTabConfig} />
      </div>
    </div>
  );
}
