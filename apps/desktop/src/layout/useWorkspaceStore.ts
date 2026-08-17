import { create } from "zustand";
import {
  WorkspaceNode,
  WorkspaceTabConfig,
  OpenChatTabInput,
  OpenFileTabInput,
  OpenTerminalTabInput,
  SplitDirection,
  LeafPaneNode,
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
import { useAppStore } from "../store/useAppStore";
import type { DropPosition } from "./WorkspaceDropzone";

export interface DraggedTabState {
  tabConfig: WorkspaceTabConfig;
  sourcePaneId?: string;
}

interface WorkspaceState {
  rootNode: WorkspaceNode;
  activePaneId: string;
  draggedTab: DraggedTabState | null;

  setDraggedTab: (dragged: DraggedTabState | null) => void;
  openChatTab: (input: OpenChatTabInput) => void;
  openFileTab: (input: OpenFileTabInput) => void;
  openTerminalTab: (input: OpenTerminalTabInput) => void;
  closeTab: (paneId: string, tabId: string) => void;
  /** Remove a tab without disposing its external resource (used when docking). */
  detachTab: (paneId: string, tabId: string) => void;
  setActiveTab: (paneId: string, tabId: string) => void;
  setActivePane: (paneId: string) => void;
  updateSplitSizes: (splitId: string, sizes: [number, number]) => void;
  splitPane: (paneId: string, direction: SplitDirection) => void;
  closePane: (paneId: string) => void;
  closeChatTab: (sessionId: string) => void;
  updateChatTabProject: (sessionId: string, newProjectId: string) => void;
  dropTabOnPane: (
    targetPaneId: string,
    position: DropPosition,
    config: WorkspaceTabConfig,
    sourcePaneId?: string,
  ) => void;
  restoreLayout: (rootNode: WorkspaceNode, activePaneId: string) => void;
}

const DEFAULT_LEAF_ID = "pane-main";

function syncTab(config?: WorkspaceTabConfig | null) {
  if (!config) return;
  if (config.type === "chat") {
    useAppStore.getState().setSelectedSessionId(config.sessionId);
    useAppStore.getState().setSelectedProjectId(config.projectId || null);
  } else if (config.projectId) {
    useAppStore.getState().setSelectedProjectId(config.projectId);
  }
}

export const useWorkspaceStore = create<WorkspaceState>((set, get) => ({
  rootNode: createLeaf(DEFAULT_LEAF_ID),
  activePaneId: DEFAULT_LEAF_ID,
  draggedTab: null,

  setDraggedTab: (draggedTab) => set({ draggedTab }),

  openChatTab: (input) => {
    const config: WorkspaceTabConfig = {
      type: "chat",
      projectId: input.projectId,
      sessionId: input.sessionId,
      title: input.title,
    };
    syncTab(config);
    set((s) => addTabToPane(s.rootNode, s.activePaneId, config));
  },

  openFileTab: (input) => {
    const config: WorkspaceTabConfig = {
      type: "file",
      projectId: input.projectId,
      path: input.path,
      title: input.title,
    };
    syncTab(config);
    set((s) => addTabToPane(s.rootNode, s.activePaneId, config));
  },

  openTerminalTab: (input) => {
    const config: WorkspaceTabConfig = {
      type: "terminal",
      projectId: input.projectId,
      terminalId: input.terminalId,
      title: input.title,
    };
    syncTab(config);
    set((s) => addTabToPane(s.rootNode, s.activePaneId, config));
  },

  setActivePane: (activePaneId) => {
    const state = get();
    const leaf = findLeaf(state.rootNode, activePaneId);
    if (leaf?.activeTabId) {
      syncTab(leaf.tabs.find((t) => getTabId(t) === leaf.activeTabId));
    }
    set({ activePaneId });
  },

  setActiveTab: (paneId, tabId) => {
    const state = get();
    const leaf = findLeaf(state.rootNode, paneId);
    if (leaf) {
      syncTab(leaf.tabs.find((t) => getTabId(t) === tabId));
    }
    set((s) => ({
      activePaneId: paneId,
      rootNode: mapTree(s.rootNode, (l) =>
        l.id === paneId ? { ...l, activeTabId: tabId } : l,
      ),
    }));
  },

  updateSplitSizes: (splitId, sizes) => {
    function update(node: WorkspaceNode): WorkspaceNode {
      if (node.type === "leaf") return node;
      const children: [WorkspaceNode, WorkspaceNode] = [
        update(node.children[0]),
        update(node.children[1]),
      ];
      const sameChildren = children[0] === node.children[0] && children[1] === node.children[1];
      const sameSizes = node.sizes[0] === sizes[0] && node.sizes[1] === sizes[1];
      if (node.id !== splitId && sameChildren) return node;
      if (node.id === splitId && sameSizes && sameChildren) return node;
      return {
        ...node,
        sizes: node.id === splitId ? sizes : node.sizes,
        children,
      };
    }

    set((s) => {
      const rootNode = update(s.rootNode);
      return rootNode === s.rootNode ? s : { rootNode };
    });
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
      syncTab(fallback);
    }

    set((s) => ({
      rootNode: mapTree(s.rootNode, (l) =>
        l.id === paneId ? { ...l, tabs: nextTabs, activeTabId: nextActiveTabId } : l,
      ),
    }));
  },

  detachTab: (paneId, tabId) => {
    const state = get();
    const leaf = findLeaf(state.rootNode, paneId);
    if (!leaf) return;

    const nextTabs = leaf.tabs.filter((t) => getTabId(t) !== tabId);
    let nextActiveTabId = leaf.activeTabId;
    if (leaf.activeTabId === tabId) {
      const closedIndex = leaf.tabs.findIndex((t) => getTabId(t) === tabId);
      const fallback = nextTabs[Math.max(0, closedIndex - 1)];
      nextActiveTabId = fallback ? getTabId(fallback) : null;
      syncTab(fallback);
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
        return {
          type: "split",
          id: `split-${Date.now()}`,
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

    function removeLeaf(node: WorkspaceNode): WorkspaceNode | null {
      if (node.type === "leaf") return node.id === paneId ? null : node;
      const left = removeLeaf(node.children[0]);
      const right = removeLeaf(node.children[1]);
      if (!left && !right) return null;
      if (!left) return right;
      if (!right) return left;
      return { ...node, children: [left, right] };
    }

    const nextRoot = removeLeaf(state.rootNode) ?? createLeaf(DEFAULT_LEAF_ID);
    const nextActive = findLeaf(nextRoot, state.activePaneId)
      ? state.activePaneId
      : (findFirstLeaf(nextRoot)?.id ?? DEFAULT_LEAF_ID);

    const activeLeaf = findLeaf(nextRoot, nextActive);
    if (activeLeaf?.activeTabId) {
      syncTab(activeLeaf.tabs.find((t) => getTabId(t) === activeLeaf.activeTabId));
    }

    set({ rootNode: nextRoot, activePaneId: nextActive });
  },

  closeChatTab: (sessionId) => {
    const targetId = `chat:${sessionId}`;
    set((s) => ({
      rootNode: mapTree(s.rootNode, (leaf) => {
        const nextTabs = leaf.tabs.filter((t) => getTabId(t) !== targetId);
        let nextActive = leaf.activeTabId;
        if (leaf.activeTabId === targetId) {
          nextActive = nextTabs[0] ? getTabId(nextTabs[0]) : null;
          syncTab(nextTabs[0]);
        }
        return { ...leaf, tabs: nextTabs, activeTabId: nextActive };
      }),
    }));
  },

  updateChatTabProject: (sessionId, newProjectId) => {
    set((s) => ({
      rootNode: mapTree(s.rootNode, (leaf) => ({
        ...leaf,
        tabs: leaf.tabs.map((t) =>
          t.type === "chat" && t.sessionId === sessionId
            ? { ...t, projectId: newProjectId }
            : t,
        ),
      })),
    }));
  },

  dropTabOnPane: (targetPaneId, position, config, sourcePaneId) => {
    const tabId = getTabId(config);
    let workingRoot = get().rootNode;

    if (sourcePaneId) {
      workingRoot = mapTree(workingRoot, (leaf) => {
        if (leaf.id !== sourcePaneId) return leaf;
        const nextTabs = leaf.tabs.filter((t) => getTabId(t) !== tabId);
        const nextActive = leaf.activeTabId === tabId ? (nextTabs[0] ? getTabId(nextTabs[0]) : null) : leaf.activeTabId;
        return { ...leaf, tabs: nextTabs, activeTabId: nextActive };
      });
    }

    syncTab(config);

    if (position === "center") {
      const res = addTabToPane(workingRoot, targetPaneId, config);
      set({ rootNode: res.rootNode, activePaneId: res.activePaneId });
      return;
    }

    const splitDirection: SplitDirection = "horizontal";
    const newLeafId = `pane-${Date.now()}`;
    const newLeaf: LeafPaneNode = {
      type: "leaf",
      id: newLeafId,
      tabs: [config],
      activeTabId: tabId,
    };

    function injectSplit(node: WorkspaceNode): WorkspaceNode {
      if (node.type === "leaf") {
        if (node.id !== targetPaneId) return node;
        const isBefore = position === "left";
        return {
          type: "split",
          id: `split-${Date.now()}`,
          direction: splitDirection,
          sizes: [50, 50],
          children: isBefore ? [newLeaf, node] : [node, newLeaf],
        };
      }
      return {
        ...node,
        children: [injectSplit(node.children[0]), injectSplit(node.children[1])],
      };
    }

    set({ rootNode: injectSplit(workingRoot), activePaneId: newLeafId });
  },

  restoreLayout: (rootNode, activePaneId) => {
    const activeLeaf = findLeaf(rootNode, activePaneId) ?? findFirstLeaf(rootNode);
    const resolvedActivePaneId = activeLeaf.id;
    const activeTab = activeLeaf.activeTabId
      ? activeLeaf.tabs.find((tab) => getTabId(tab) === activeLeaf.activeTabId)
      : undefined;
    syncTab(activeTab);
    set({ rootNode, activePaneId: resolvedActivePaneId });
  },
}));
