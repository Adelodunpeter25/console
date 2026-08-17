import type { FileTabConfig, WorkspaceNode, WorkspaceTabConfig } from "./types";
import { addTabToPane, findLeaf, mapTree } from "./treeHelpers";
import { getTabId } from "./types";

const FILE_PREVIEW_WINDOW_MS = 5 * 60 * 1000;

export function openFileTabInWorkspace(
  rootNode: WorkspaceNode,
  activePaneId: string,
  config: FileTabConfig,
): { rootNode: WorkspaceNode; activePaneId: string } {
  const existingFile = findFileTab(rootNode, config.path);
  if (existingFile) {
    return addTabToPane(rootNode, activePaneId, config);
  }

  const activeLeaf = findLeaf(rootNode, activePaneId);
  const now = Date.now();
  const recentPreview = activeLeaf
    ? activeLeaf.tabs
        .filter((tab): tab is FileTabConfig => tab.type === "file")
        .filter(
          (tab) =>
            typeof tab.openedAt === "number" &&
            now - tab.openedAt < FILE_PREVIEW_WINDOW_MS,
        )
        .sort((left, right) => (right.openedAt ?? 0) - (left.openedAt ?? 0))[0]
    : undefined;

  if (!recentPreview || !activeLeaf) {
    return addTabToPane(rootNode, activePaneId, config);
  }

  const recentTabId = getTabId(recentPreview);
  const nextTabId = getTabId(config);
  return {
    rootNode: mapTree(rootNode, (leaf) =>
      leaf.id === activeLeaf.id
        ? {
            ...leaf,
            tabs: leaf.tabs.map((tab) => (getTabId(tab) === recentTabId ? config : tab)),
            activeTabId: nextTabId,
          }
        : leaf,
    ),
    activePaneId: activeLeaf.id,
  };
}

function findFileTab(rootNode: WorkspaceNode, path: string): WorkspaceTabConfig | undefined {
  if (rootNode.type === "leaf") {
    return rootNode.tabs.find((tab) => tab.type === "file" && tab.path === path);
  }
  return findFileTab(rootNode.children[0], path) ?? findFileTab(rootNode.children[1], path);
}
