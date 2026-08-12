import { create } from "zustand";
import {
  WorkspaceNode,
  WorkspaceTabConfig,
  OpenChatTabInput,
  OpenFileTabInput,
  OpenTerminalTabInput,
  SplitDirection,
  getTabId,
} from "./types";
import {
  createLeaf,
  findLeaf,
  findFirstLeaf,
  mapTree,
  addTabToPane,
} from "./treeHelpers";
import { useTerminalStore } from "../store/useTerminalStore";
import type { DropPosition } from "./WorkspaceDropzone";

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
  dropTabOnPane: (
    targetPaneId: string,
    position: DropPosition,
    config: WorkspaceTabConfig,
    sourcePaneId?: string,
  ) => void;
}

const DEFAULT_LEAF_ID = "pane-main";

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

  dropTabOnPane: (targetPaneId, position, config, sourcePaneId) => {
    const state = get();
    const tabId = getTabId(config);

    let currentTree = state.rootNode;
    if (sourcePaneId) {
      currentTree = mapTree(currentTree, (leaf) => {
        if (leaf.id !== sourcePaneId) return leaf;
        const nextTabs = leaf.tabs.filter((t) => getTabId(t) !== tabId);
        const nextActive =
          leaf.activeTabId === tabId ? (nextTabs[0] ? getTabId(nextTabs[0]) : null) : leaf.activeTabId;
        return { ...leaf, tabs: nextTabs, activeTabId: nextActive };
      });
    }

    if (position === "center") {
      const added = addTabToPane(currentTree, targetPaneId, config);
      set({ rootNode: added.rootNode, activePaneId: added.activePaneId });
      return;
    }

    const direction: SplitDirection =
      position === "left" || position === "right" ? "horizontal" : "vertical";
    const targetLeaf = findLeaf(currentTree, targetPaneId);
    if (!targetLeaf) {
      const added = addTabToPane(currentTree, targetPaneId, config);
      set({ rootNode: added.rootNode, activePaneId: added.activePaneId });
      return;
    }

    const newLeafId = `pane-${Date.now()}`;
    const newLeaf = createLeaf(newLeafId, [config], tabId);

    function splitAndDrop(node: WorkspaceNode): WorkspaceNode {
      if (node.type === "leaf") {
        if (node.id !== targetPaneId) return node;
        const splitId = `split-${Date.now()}`;
        const isLeftOrTop = position === "left" || position === "top";
        const children: [WorkspaceNode, WorkspaceNode] = isLeftOrTop
          ? [newLeaf, node]
          : [node, newLeaf];
        return {
          type: "split",
          id: splitId,
          direction,
          sizes: [50, 50],
          children,
        };
      }
      return {
        ...node,
        children: [splitAndDrop(node.children[0]), splitAndDrop(node.children[1])],
      };
    }

    set({
      rootNode: splitAndDrop(currentTree),
      activePaneId: newLeafId,
    });
  },
}));
