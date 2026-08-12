import { create } from "zustand";
import {
  WorkspaceNode,
  LeafPaneNode,
  WorkspaceTabConfig,
  OpenChatTabInput,
  OpenFileTabInput,
  OpenTerminalTabInput,
  SplitDirection,
  getTabId,
} from "./types";
import { useTerminalStore } from "../store/useTerminalStore";

interface WorkspaceState {
  rootNode: WorkspaceNode;
  activePaneId: string;

  openChatTab: (input: OpenChatTabInput) => void;
  openFileTab: (input: OpenFileTabInput) => void;
  openTerminalTab: (input: OpenTerminalTabInput) => void;
  closeTab: (paneId: string, tabId: string) => void;
  setActiveTab: (paneId: string, tabId: string) => void;
  setActivePane: (paneId: string) => void;
  splitPane: (paneId: string, direction: SplitDirection) => void;
  closePane: (paneId: string) => void;
  closeChatTab: (projectId: string, sessionId: string) => void;
  updateChatTabProject: (sessionId: string, newProjectId: string) => void;
}

const DEFAULT_LEAF_ID = "pane-main";

function createLeaf(id: string, tabs: WorkspaceTabConfig[] = [], activeTabId: string | null = null): LeafPaneNode {
  return { type: "leaf", id, tabs, activeTabId: activeTabId ?? (tabs[0] ? getTabId(tabs[0]) : null) };
}

function findLeaf(node: WorkspaceNode, paneId: string): LeafPaneNode | null {
  if (node.type === "leaf") return node.id === paneId ? node : null;
  return findLeaf(node.children[0], paneId) || findLeaf(node.children[1], paneId);
}

function findFirstLeaf(node: WorkspaceNode): LeafPaneNode {
  if (node.type === "leaf") return node;
  return findFirstLeaf(node.children[0]);
}

function mapTree(node: WorkspaceNode, fn: (leaf: LeafPaneNode) => LeafPaneNode): WorkspaceNode {
  if (node.type === "leaf") return fn(node);
  return {
    ...node,
    children: [mapTree(node.children[0], fn), mapTree(node.children[1], fn)],
  };
}

function addTabToPane(
  tree: WorkspaceNode,
  targetPaneId: string,
  config: WorkspaceTabConfig,
): { rootNode: WorkspaceNode; activePaneId: string } {
  const tabId = getTabId(config);

  // 1. Check if tab is already open in ANY leaf pane
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

  // 2. Tab is not open anywhere: add it to targetPaneId (or fallback to first leaf)
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

export const useWorkspaceStore = create<WorkspaceState>((set, get) => ({
  rootNode: createLeaf(DEFAULT_LEAF_ID),
  activePaneId: DEFAULT_LEAF_ID,

  openChatTab: (input) => {
    const config: WorkspaceTabConfig = {
      type: "chat",
      projectId: input.projectId,
      sessionId: input.sessionId,
      title: input.title,
    };
    set((s) => addTabToPane(s.rootNode, s.activePaneId, config));
  },

  openFileTab: (input) => {
    const config: WorkspaceTabConfig = {
      type: "file",
      projectId: input.projectId,
      path: input.path,
      title: input.title,
    };
    set((s) => addTabToPane(s.rootNode, s.activePaneId, config));
  },

  openTerminalTab: (input) => {
    const config: WorkspaceTabConfig = {
      type: "terminal",
      projectId: input.projectId,
      terminalId: input.terminalId,
      title: input.title,
    };
    set((s) => addTabToPane(s.rootNode, s.activePaneId, config));
  },

  setActivePane: (activePaneId) => set({ activePaneId }),

  setActiveTab: (paneId, tabId) => {
    set((s) => ({
      activePaneId: paneId,
      rootNode: mapTree(s.rootNode, (leaf) =>
        leaf.id === paneId ? { ...leaf, activeTabId: tabId } : leaf,
      ),
    }));
  },

  closeTab: (paneId, tabId) => {
    const state = get();
    const leaf = findLeaf(state.rootNode, paneId);
    if (!leaf) return;

    const closedTab = leaf.tabs.find((t) => getTabId(t) === tabId);
    if (closedTab && closedTab.type === "terminal") {
      void useTerminalStore.getState().kill(closedTab.terminalId);
    }

    const nextTabs = leaf.tabs.filter((t) => getTabId(t) !== tabId);
    let nextActiveTabId = leaf.activeTabId;
    if (leaf.activeTabId === tabId) {
      const closedIndex = leaf.tabs.findIndex((t) => getTabId(t) === tabId);
      const fallback = nextTabs[Math.max(0, closedIndex - 1)];
      nextActiveTabId = fallback ? getTabId(fallback) : null;
    }

    set((s) => ({
      rootNode: mapTree(s.rootNode, (l) =>
        l.id === paneId ? { ...l, tabs: nextTabs, activeTabId: nextActiveTabId } : l,
      ),
    }));
  },

  splitPane: (paneId, direction) => {
    const state = get();
    const targetLeaf = findLeaf(state.rootNode, paneId);
    if (!targetLeaf) return;

    const newLeafId = `pane-${Date.now()}`;
    const newLeaf = createLeaf(newLeafId);

    function splitNode(node: WorkspaceNode): WorkspaceNode {
      if (node.type === "leaf") {
        if (node.id !== paneId) return node;
        const splitId = `split-${Date.now()}`;
        return {
          type: "split",
          id: splitId,
          direction,
          sizes: [50, 50],
          children: [node, newLeaf],
        };
      }
      return {
        ...node,
        children: [splitNode(node.children[0]), splitNode(node.children[1])],
      };
    }

    set((s) => ({
      rootNode: splitNode(s.rootNode),
      activePaneId: newLeafId,
    }));
  },

  closePane: (paneId) => {
    const state = get();
    if (state.rootNode.type === "leaf") return;

    function removeNode(node: WorkspaceNode): WorkspaceNode | null {
      if (node.type === "leaf") {
        return node.id === paneId ? null : node;
      }
      const left = removeNode(node.children[0]);
      const right = removeNode(node.children[1]);
      if (!left) return right;
      if (!right) return left;
      return { ...node, children: [left, right] };
    }

    const nextRoot = removeNode(state.rootNode);
    if (!nextRoot) return;

    const firstLeaf = findFirstLeaf(nextRoot);
    set({ rootNode: nextRoot, activePaneId: firstLeaf.id });
  },

  closeChatTab: (_projectId, sessionId) => {
    const tabId = `chat:${sessionId}`;
    const state = get();
    mapTree(state.rootNode, (leaf) => {
      if (leaf.tabs.some((t) => getTabId(t) === tabId)) {
        state.closeTab(leaf.id, tabId);
      }
      return leaf;
    });
  },

  updateChatTabProject: (sessionId, newProjectId) => {
    const tabId = `chat:${sessionId}`;
    set((s) => ({
      rootNode: mapTree(s.rootNode, (leaf) => {
        const nextTabs = leaf.tabs.map((t) =>
          getTabId(t) === tabId && t.type === "chat"
            ? { ...t, projectId: newProjectId }
            : t,
        );
        return { ...leaf, tabs: nextTabs };
      }),
    }));
  },
}));
