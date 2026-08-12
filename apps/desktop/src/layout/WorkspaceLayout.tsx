import React from "react";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { WorkspacePane } from "./WorkspacePane";

/**
 * WorkspaceLayout — Top-level shell layout container for cmux-style tiling workspace.
 * Listens to global keyboard shortcuts (⌘D for split right, ⌘Shift+D for split down)
 * and renders the root WorkspacePane.
 */
export function WorkspaceLayout() {
  const rootNode = useWorkspaceStore((state) => state.rootNode);
  const activePaneId = useWorkspaceStore((state) => state.activePaneId);
  const splitPane = useWorkspaceStore((state) => state.splitPane);

  React.useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
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
  }, [activePaneId, splitPane]);

  return (
    <main className="w-full h-full overflow-hidden bg-screen">
      <WorkspacePane node={rootNode} />
    </main>
  );
}