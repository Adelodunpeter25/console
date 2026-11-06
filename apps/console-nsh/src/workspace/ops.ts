// Pure operations on the workspace pane tree, ported from console-ui's
// `workspace/ops.rs` (which calls itself "the Rust equivalent of the desktop
// app's useWorkspaceStore actions"). They take and return the tree in place,
// so the app state stays a plain owner; UI callbacks call these and notify.

import {
  leafNode,
  leaves,
  leafById,
  splitNode,
  tabId,
  type LeafPaneNode,
  type SplitDirection,
  type WorkspaceNode,
  type WorkspaceTabConfig,
} from "./model";

/// A short unique suffix for generated pane/split ids.
let uniqueCounter = 0;
function uniqueSuffix(): string {
  return (++uniqueCounter).toString(16);
}

/// The tree is plain JSON data, so a JSON round-trip clones it (the runtime
/// has no `structuredClone`).
function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

/// The pane a tab lands in: the active pane, or the first leaf when there are
/// no panes yet.
export function activeLeaf(
  root: WorkspaceNode,
  activePaneId: string | null,
): LeafPaneNode {
  const known =
    activePaneId != null && leafById(root, activePaneId) != null
      ? activePaneId
      : (leaves(root)[0]?.id ?? null);
  if (known != null) return leafById(root, known)!;
  // No leaves at all: swap the root for a fresh default leaf.
  Object.assign(root, leafNode("pane-main"));
  return leafById(root, "pane-main")!;
}

/// Open a tab in the given pane (creating the pane when needed). If the tab
/// is already open there, it is only activated.
export function openTab(
  root: WorkspaceNode,
  paneId: string,
  tab: WorkspaceTabConfig,
): void {
  const leaf = activeLeaf(root, paneId);
  const id = tabId(tab);
  if (!leaf.tabs.some((t) => tabId(t) === id)) leaf.tabs.push(tab);
  leaf.activeTabId = id;
}

/// Add a tab to a pane without changing which tab is active. If the pane has
/// no active tab yet, the new tab becomes active.
export function openTabInBackground(
  root: WorkspaceNode,
  paneId: string,
  tab: WorkspaceTabConfig,
): void {
  const leaf = activeLeaf(root, paneId);
  const id = tabId(tab);
  if (!leaf.tabs.some((t) => tabId(t) === id)) leaf.tabs.push(tab);
  if (leaf.activeTabId == null) leaf.activeTabId = id;
}

/// Open a tab, or replace an existing preview tab if found.
/// Replacement stays within its kind: a file preview replaces only a file
/// tab, a diff preview only a diff tab (a rejected replace falls through to
/// opening a new tab). Returns the id of the tab now shown.
export function replaceOrOpenTab(
  root: WorkspaceNode,
  paneId: string,
  replaceTabId: string | null,
  newTab: WorkspaceTabConfig,
): string {
  const leaf = activeLeaf(root, paneId);
  const newId = tabId(newTab);

  if (replaceTabId != null) {
    const pos = leaf.tabs.findIndex((t) => tabId(t) === replaceTabId);
    if (pos >= 0 && leaf.tabs[pos]!.type === newTab.type) {
      leaf.tabs[pos] = newTab;
      leaf.activeTabId = newId;
      return newId;
    }
  }

  if (!leaf.tabs.some((t) => tabId(t) === newId)) leaf.tabs.push(newTab);
  leaf.activeTabId = newId;
  return newId;
}

/// Move a tab into an existing pane, removing it from its source first.
/// Existing tabs are deduplicated and activated instead of duplicated.
export function moveTabToPane(
  root: WorkspaceNode,
  sourcePaneId: string | null,
  targetPaneId: string,
  tab: WorkspaceTabConfig,
): string | null {
  if (leafById(root, targetPaneId) == null) return null;

  const id = tabId(tab);
  if (sourcePaneId !== targetPaneId && sourcePaneId != null) {
    closeTab(root, sourcePaneId, id);
  }
  openTab(root, targetPaneId, tab);
  return targetPaneId;
}

/// Close a tab in a pane, activating the neighbor (left-first, like the
/// desktop app). Returns the id of the tab that became active, if any.
export function closeTab(
  root: WorkspaceNode,
  paneId: string,
  tabId_: string,
): string | null {
  const leaf = leafById(root, paneId);
  if (leaf == null) return null;
  const closedIndex = leaf.tabs.findIndex((t) => tabId(t) === tabId_);
  if (closedIndex < 0) return null;
  leaf.tabs.splice(closedIndex, 1);

  if (leaf.activeTabId === tabId_) {
    const fallback =
      leaf.tabs[Math.max(0, closedIndex - 1)] ?? leaf.tabs[0] ?? null;
    leaf.activeTabId = fallback ? tabId(fallback) : null;
  }
  return leaf.activeTabId;
}

/// Remove a pane and collapse its parent split into the remaining sibling.
/// The final pane is never removed, so the workspace always has a host for a
/// new chat.
export function closePane(root: WorkspaceNode, paneId: string): boolean {
  if (leaves(root).length <= 1) return false;

  function removeFrom(node: WorkspaceNode, paneId: string): boolean {
    if (node.type !== "split") return false;

    for (const childIndex of [0, 1] as const) {
      const child = node.children[childIndex]!;
      if (!leaves(child).some((leaf) => leaf.id === paneId)) continue;

      if (child.type === "leaf" && child.id === paneId) {
        Object.assign(node, node.children[1 - childIndex]!);
        return true;
      }
      if (removeFrom(child, paneId)) return true;
    }
    return false;
  }

  return removeFrom(root, paneId);
}

/// Activate a tab in a pane.
export function selectTab(
  root: WorkspaceNode,
  paneId: string,
  tabId_: string,
): void {
  const leaf = leafById(root, paneId);
  if (leaf?.tabs.some((t) => tabId(t) === tabId_)) leaf.activeTabId = tabId_;
}

/// Remove the tab with `tabId_` from wherever it lives in the tree,
/// fixing up active ids. Returns the removed tab, if any.
export function takeTab(
  root: WorkspaceNode,
  tabId_: string,
): WorkspaceTabConfig | null {
  let removed: WorkspaceTabConfig | null = null;
  const toClose: [string, string][] = [];
  for (const leaf of leaves(root)) {
    for (const tab of leaf.tabs) {
      if (tabId(tab) === tabId_) {
        removed ??= tab;
        toClose.push([leaf.id, tabId(tab)]);
      }
    }
  }
  for (const [paneId, tid] of toClose) closeTab(root, paneId, tid);
  return removed;
}

/// Drop every Settings tab from the tree.
///
/// The settings are no longer a tab: they take over the window. The variant
/// stays in the workspace model so a workspace saved by an older build still
/// parses, but a restored one must not bring the old tab back.
export function retainNonSettingsTabs(root: WorkspaceNode): void {
  const toClose: [string, string][] = [];
  for (const leaf of leaves(root)) {
    for (const tab of leaf.tabs) {
      if (tab.type === "settings") toClose.push([leaf.id, tabId(tab)]);
    }
  }
  for (const [paneId, tid] of toClose) closeTab(root, paneId, tid);
}

/// Find a tab by id anywhere in the tree.
export function findTab(
  root: WorkspaceNode,
  tabId_: string,
): WorkspaceTabConfig | null {
  for (const leaf of leaves(root)) {
    for (const tab of leaf.tabs) {
      if (tabId(tab) === tabId_) return tab;
    }
  }
  return null;
}

/// Split a leaf pane into two, keeping the original leaf and adding a fresh
/// empty one beside it. Returns the id of the new pane.
export function splitPane(
  root: WorkspaceNode,
  paneId: string,
  direction: SplitDirection,
): string | null {
  const newId = `pane-${uniqueSuffix()}`;
  const splitId = `split-${uniqueSuffix()}`;
  const newLeaf = leafNode(newId);

  function inject(
    node: WorkspaceNode,
    paneId: string,
    splitId: string,
    direction: SplitDirection,
    newLeaf: WorkspaceNode,
  ): boolean {
    if (node.type === "leaf") {
      if (node.id !== paneId) return false;
      Object.assign(
        node,
        splitNode(splitId, direction, clone(node), newLeaf),
      );
      return true;
    }
    return (
      inject(node.children[0]!, paneId, splitId, direction, clone(newLeaf)) ||
      inject(node.children[1]!, paneId, splitId, direction, newLeaf)
    );
  }

  return inject(root, paneId, splitId, direction, newLeaf) ? newId : null;
}

/// Move a tab into a new split adjacent to `targetPaneId`.
///
/// `insertFirst` puts the dragged tab's pane before the target pane (left
/// for `horizontal`, above for `vertical`). The source pane is optional so
/// sidebar sessions can be dropped into a new pane without first existing in
/// the workspace tree.
export function moveTabToSplit(
  root: WorkspaceNode,
  sourcePaneId: string | null,
  targetPaneId: string,
  tab: WorkspaceTabConfig,
  direction: SplitDirection,
  insertFirst: boolean,
): string | null {
  if (leafById(root, targetPaneId) == null) return null;

  const id = tabId(tab);
  if (sourcePaneId != null) closeTab(root, sourcePaneId, id);

  const newPaneId = `pane-${uniqueSuffix()}`;
  const splitId = `split-${uniqueSuffix()}`;
  const newLeaf: WorkspaceNode = {
    type: "leaf",
    id: newPaneId,
    tabs: [tab],
    activeTabId: id,
  };

  function inject(
    node: WorkspaceNode,
    targetPaneId: string,
    splitId: string,
    direction: SplitDirection,
    insertFirst: boolean,
    newLeaf: WorkspaceNode,
  ): boolean {
    if (node.type === "leaf") {
      if (node.id !== targetPaneId) return false;
      const original = clone(node);
      Object.assign(
        node,
        insertFirst
          ? splitNode(splitId, direction, newLeaf, original)
          : splitNode(splitId, direction, original, newLeaf),
      );
      return true;
    }
    return (
      inject(
        node.children[0]!,
        targetPaneId,
        splitId,
        direction,
        insertFirst,
        clone(newLeaf),
      ) ||
      inject(node.children[1]!, targetPaneId, splitId, direction, insertFirst, newLeaf)
    );
  }

  return inject(root, targetPaneId, splitId, direction, insertFirst, newLeaf)
    ? newPaneId
    : null;
}

/// Update the proportion of a split node given a new size for the first
/// child (in percent, clamped to 10..=90).
export function resizeSplit(
  root: WorkspaceNode,
  splitId: string,
  size0: number,
): boolean {
  const clamped = Math.min(90.0, Math.max(10.0, size0));
  const sizes: [number, number] = [clamped, 100.0 - clamped];

  function updateSplit(node: WorkspaceNode, splitId: string, sizes: [number, number]): boolean {
    if (node.type !== "split") return false;
    if (node.id === splitId) {
      node.sizes = sizes;
      return true;
    }
    return (
      updateSplit(node.children[0]!, splitId, sizes) ||
      updateSplit(node.children[1]!, splitId, sizes)
    );
  }

  return updateSplit(root, splitId, sizes);
}

/// Find current sizes for a split node.
export function findSplitSizes(
  root: WorkspaceNode,
  splitId: string,
): [number, number] | null {
  if (root.type !== "split") return null;
  if (root.id === splitId) return root.sizes;
  return (
    findSplitSizes(root.children[0]!, splitId) ??
    findSplitSizes(root.children[1]!, splitId)
  );
}
