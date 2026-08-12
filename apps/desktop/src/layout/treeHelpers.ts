import {
  WorkspaceNode,
  LeafPaneNode,
  WorkspaceTabConfig,
  getTabId,
} from "./types";

export function createLeaf(
  id: string,
  tabs: WorkspaceTabConfig[] = [],
  activeTabId: string | null = null,
): LeafPaneNode {
  return { type: "leaf", id, tabs, activeTabId: activeTabId ?? (tabs[0] ? getTabId(tabs[0]) : null) };
}

export function findLeaf(node: WorkspaceNode, paneId: string): LeafPaneNode | null {
  if (node.type === "leaf") return node.id === paneId ? node : null;
  return findLeaf(node.children[0], paneId) || findLeaf(node.children[1], paneId);
}

export function findFirstLeaf(node: WorkspaceNode): LeafPaneNode {
  if (node.type === "leaf") return node;
  return findFirstLeaf(node.children[0]);
}

export function mapTree(node: WorkspaceNode, fn: (leaf: LeafPaneNode) => LeafPaneNode): WorkspaceNode {
  if (node.type === "leaf") return fn(node);
  return {
    ...node,
    children: [mapTree(node.children[0], fn), mapTree(node.children[1], fn)],
  };
}

export function addTabToPane(
  tree: WorkspaceNode,
  targetPaneId: string,
  config: WorkspaceTabConfig,
): { rootNode: WorkspaceNode; activePaneId: string } {
  const tabId = getTabId(config);

  let existingPaneId: string | null = null;
  function searchTab(node: WorkspaceNode) {
    if (node.type === "leaf") {
      if (node.tabs.some((t) => getTabId(t) === tabId)) {
        existingPaneId = node.id;
      }
    } else {
      searchTab(node.children[0]);
      searchTab(node.children[1]);
    }
  }
  searchTab(tree);

  if (existingPaneId) {
    const nextTree = mapTree(tree, (leaf) =>
      leaf.id === existingPaneId
        ? {
            ...leaf,
            tabs: leaf.tabs.map((t) => (getTabId(t) === tabId ? config : t)),
            activeTabId: tabId,
          }
        : leaf,
    );
    return { rootNode: nextTree, activePaneId: existingPaneId };
  }

  let paneFound = false;
  const nextTree = mapTree(tree, (leaf) => {
    if (leaf.id !== targetPaneId) return leaf;
    paneFound = true;
    return { ...leaf, tabs: [...leaf.tabs, config], activeTabId: tabId };
  });

  if (paneFound) return { rootNode: nextTree, activePaneId: targetPaneId };

  const first = findFirstLeaf(tree);
  return addTabToPane(tree, first.id, config);
}
