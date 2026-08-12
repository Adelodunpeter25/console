import React from "react";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { WorkspacePane } from "./WorkspacePane";
import { WorkspaceNode, LeafPaneNode } from "./types";

function findLeaf(node: WorkspaceNode, paneId: string): LeafPaneNode | null {
  if (node.type === "leaf") return node.id === paneId ? node : null;
  return findLeaf(node.children[0], paneId) || findLeaf(node.children[1], paneId);
}

/**
 * WorkspaceLayout — Top-level shell layout container for cmux-style tiling workspace.
 * Listens to global keyboard shortcuts:
 * - ⌘W / Ctrl+W: Close active tab in active pane
 * - ⌘D / Ctrl+D: Split active pane Right (horizontal)
 * - ⌘Shift+D / Ctrl+Shift+D: Split active pane Down (vertical)
 */
export function WorkspaceLayout() {
  const rootNode = useWorkspaceStore((state) => state.rootNode);
  const activePaneId = useWorkspaceStore((state) => state.activePaneId);
  const splitPane = useWorkspaceStore((state) => state.splitPane);
  const closeTab = useWorkspaceStore((state) => state.closeTab);

  React.useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // ⌘W / Ctrl+W -> Close active tab
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "w") {
        e.preventDefault();
        const leaf = findLeaf(rootNode, activePaneId);
        if (leaf && leaf.activeTabId) {
          closeTab(activePaneId, leaf.activeTabId);
        }
      }

      // ⌘D / Ctrl+D -> Split pane
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "d") {
        e.preventDefault();
        if (e.shiftKey) {
          splitPane(activePaneId, "vertical");
        } else {
          splitPane(activePaneId, "horizontal");
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [rootNode, activePaneId, splitPane, closeTab]);

  return (
    <main className="w-full h-full overflow-hidden bg-black">
      <WorkspacePane node={rootNode} />
    </main>
  );
}