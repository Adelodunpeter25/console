// The workspace store: signal-based state over the pure pane-tree ops —
// the TSX twin of the GPUI app's `state/workspace_panes.rs` actions. The
// tree itself is plain data (see ./model); this owner adds reactivity, the
// active-pane cursor, and tab bookkeeping. Components read what they need;
// each mutation bumps exactly one signal, so nsh re-renders only the
// subscribers.

import { signal } from "nsh";
import {
  firstLeaf,
  leafById,
  leafNode,
  leaves,
  tabId,
  type LeafPaneNode,
  type SplitDirection,
  type WorkspaceNode,
  type WorkspaceTabConfig,
} from "./model";
import {
  activeLeaf,
  closePane as closePaneOp,
  closeTab as closeTabOp,
  moveTabToPane as moveTabToPaneOp,
  moveTabToSplit as moveTabToSplitOp,
  openTab as openTabOp,
  openTabInBackground as openTabInBackgroundOp,
  replaceOrOpenTab as replaceOrOpenTabOp,
  resizeSplit as resizeSplitOp,
  selectTab as selectTabOp,
  splitPane as splitPaneOp,
  takeTab as takeTabOp,
} from "./ops";

export function createWorkspaceStore(root?: WorkspaceNode) {
  const tree = signal<WorkspaceNode>(root ?? leafNode("pane-main"));
  const activePaneId = signal<string | null>(null);
  // Bumped on every mutation, for views that just need "something changed".
  const revision = signal(0);

  function paneChanged(paneId: string | null): void {
    if (paneId != null) activePaneId.set(paneId);
    revision.set(revision() + 1);
  }

  /// The pane id a new tab lands in when the caller doesn't name one: the
  /// active pane, else the first leaf. The ops' activeLeaf guarantees a leaf
  /// exists (swapping in a fresh default pane for a degenerate tree).
  function targetPaneId(paneId: string | null): string {
    if (paneId != null) return paneId;
    return activePaneId() ?? firstLeaf(tree())!.id;
  }

  return {
    tree,
    activePaneId,
    revision,

    panes(): LeafPaneNode[] {
      return leaves(tree());
    },

    activePane(): LeafPaneNode {
      return activeLeaf(tree(), activePaneId());
    },

    /// Opens a tab in a pane (or the active one), creating panes as needed.
    openTab(paneId: string | null, tab: WorkspaceTabConfig): string {
      const target = targetPaneId(paneId);
      openTabOp(tree(), target, tab);
      paneChanged(target);
      return target;
    },

    /// Adds a tab without changing which tab is active.
    openTabInBackground(paneId: string | null, tab: WorkspaceTabConfig): void {
      openTabInBackgroundOp(tree(), targetPaneId(paneId), tab);
      revision.set(revision() + 1);
    },

    /// Replaces an open preview tab or opens a new one, returning the id shown.
    replaceOrOpenTab(
      paneId: string | null,
      replaceTabId: string | null,
      tab: WorkspaceTabConfig,
    ): string {
      const target = targetPaneId(paneId);
      const id = replaceOrOpenTabOp(tree(), target, replaceTabId, tab);
      paneChanged(target);
      return id;
    },

    /// Closes a tab, and its pane too when the pane is left empty (the final
    /// pane always stays). Returns the id of the now-active pane.
    closeTab(paneId: string, tab: WorkspaceTabConfig): string {
      closeTabOp(tree(), paneId, tabId(tab));
      const leaf = leafById(tree(), paneId);
      if (leaf != null && leaf.tabs.length === 0 && leaves(tree()).length > 1) {
        closePaneOp(tree(), paneId);
      }
      paneChanged(null);
      return activePaneId() ?? firstLeaf(tree())!.id;
    },

    selectTab(paneId: string, tab: WorkspaceTabConfig): void {
      selectTabOp(tree(), paneId, tabId(tab));
      paneChanged(paneId);
    },

    focusPane(paneId: string): void {
      if (leafById(tree(), paneId) != null) paneChanged(paneId);
    },

    /// Splits a pane in two, keeping the original and adding a fresh empty
    /// pane beside it. Returns the new pane's id, or null when `paneId` is
    /// not in the tree.
    splitPane(paneId: string, direction: SplitDirection): string | null {
      const newId = splitPaneOp(tree(), paneId, direction);
      if (newId != null) paneChanged(null);
      return newId;
    },

    resizeSplit(splitId: string, size0: number): boolean {
      const ok = resizeSplitOp(tree(), splitId, size0);
      if (ok) revision.set(revision() + 1);
      return ok;
    },

    /// Moves a tab between existing panes (deduped, activated at the target).
    moveTabToPane(
      sourcePaneId: string | null,
      targetPaneId: string,
      tab: WorkspaceTabConfig,
    ): void {
      const moved = moveTabToPaneOp(tree(), sourcePaneId, targetPaneId, tab);
      if (moved != null) paneChanged(targetPaneId);
    },

    /// Moves a tab into a fresh split beside `targetPaneId`.
    moveTabToSplit(
      sourcePaneId: string | null,
      targetPaneId: string,
      tab: WorkspaceTabConfig,
      direction: SplitDirection,
      insertFirst: boolean,
    ): string | null {
      const newPaneId = moveTabToSplitOp(
        tree(),
        sourcePaneId,
        targetPaneId,
        tab,
        direction,
        insertFirst,
      );
      if (newPaneId != null) paneChanged(newPaneId);
      return newPaneId;
    },

    /// Removes a tab from the whole tree. Returns the removed tab, if any.
    takeTab(tabId_: string): WorkspaceTabConfig | null {
      const removed = takeTabOp(tree(), tabId_);
      if (removed != null) paneChanged(null);
      return removed;
    },

    /// Swaps in a restored tree (app start; the persisted document parses
    /// back to the same shape).
    restoreTree(restored: WorkspaceNode): void {
      tree.set(restored);
      paneChanged(null);
    },
  };
}

export type WorkspaceStore = ReturnType<typeof createWorkspaceStore>;
