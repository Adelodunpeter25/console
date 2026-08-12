import React from "react";
import { Group, Panel, Separator } from "react-resizable-panels";
import { WorkspaceNode, LeafPaneNode, SplitPaneNode, getTabId } from "./types";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { WorkspaceTabBar } from "./WorkspaceTabBar";
import { WorkspaceContent } from "./WorkspaceContent";

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

  const activeTabConfig = React.useMemo(() => {
    if (!node.activeTabId) return null;
    return node.tabs.find((t) => getTabId(t) === node.activeTabId) ?? null;
  }, [node.tabs, node.activeTabId]);

  return (
    <div
      onClick={() => setActivePane(node.id)}
      className="flex flex-col h-full w-full overflow-hidden bg-black"
    >
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
