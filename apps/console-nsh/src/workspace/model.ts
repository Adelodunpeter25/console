// The workspace pane-tree model, ported from console-core's
// `types/workspace.rs`: a recursive tree of split nodes (resizable
// side-by-side panes) and leaf nodes (a tab strip plus the active tab).
// Pure data, so the tree persists and rebuilds across launches, and the
// operations in `./ops` stay plain functions over it.

/// One tab inside a workspace leaf. Each tab names a view: a chat session, a
/// terminal, a file, a diff, a browser page, a changes review, or settings.
export type WorkspaceTabConfig =
  | { type: "chat"; sessionId: string; title: string; provider?: string | null; projectId?: string | null; lastActiveAtMs?: number | null }
  | { type: "terminal"; terminalId: string; title: string; projectId?: string | null; lastActiveAtMs?: number | null }
  | { type: "file"; path: string; title: string; projectId?: string | null; lastActiveAtMs?: number | null }
  | { type: "diff"; path: string; title: string; projectId?: string | null; lastActiveAtMs?: number | null }
  | { type: "browser"; browserId: string; url: string; title: string; sessionId?: string | null; projectId?: string | null; lastActiveAtMs?: number | null }
  | { type: "changesReview"; sessionId: string; scope: string; title: string; projectId?: string | null; lastActiveAtMs?: number | null }
  | { type: "settings"; title?: string; activeTab?: string | null; lastActiveAtMs?: number | null };

/// Stable identity used to compare and select tabs (`chat:<id>`, …).
export function tabId(tab: WorkspaceTabConfig): string {
  switch (tab.type) {
    case "chat":
      return `chat:${tab.sessionId}`;
    case "terminal":
      return `term:${tab.terminalId}`;
    case "file":
      return `file:${tab.path}`;
    case "diff":
      return `diff:${tab.path}`;
    case "browser":
      return `browser:${tab.browserId}`;
    case "changesReview":
      return `changesReview:${tab.sessionId}:${tab.scope}`;
    case "settings":
      return "settings";
  }
}

export function tabTitle(tab: WorkspaceTabConfig): string {
  return tab.type === "settings" ? (tab.title ?? "Settings") : tab.title;
}

export function setTabTitle(tab: WorkspaceTabConfig, title: string): void {
  if (tab.type === "settings") tab.title = title;
  else tab.title = title;
}

export function tabProjectId(tab: WorkspaceTabConfig): string | null {
  return tab.type === "settings" ? null : (tab.projectId ?? null);
}

export function setTabProjectId(
  tab: WorkspaceTabConfig,
  projectId: string | null,
): void {
  if (tab.type !== "settings") tab.projectId = projectId;
}

export function tabLastActiveAtMs(tab: WorkspaceTabConfig): number | null {
  return tab.lastActiveAtMs ?? null;
}

export function setTabLastActiveAtMs(
  tab: WorkspaceTabConfig,
  tsMs: number | null,
): void {
  tab.lastActiveAtMs = tsMs;
}

/// A leaf pane: a tab strip plus the currently active tab.
export interface LeafPaneNode {
  id: string;
  tabs: WorkspaceTabConfig[];
  activeTabId: string | null;
}

export type SplitDirection = "horizontal" | "vertical";

/// A split between two panes, with proportional sizes in percent.
export interface SplitPaneNode {
  id: string;
  /// `horizontal` = side-by-side columns; `vertical` = stacked rows.
  direction: SplitDirection;
  /// Percentage sizes of the two children.
  sizes: [number, number];
  children: [WorkspaceNode, WorkspaceNode];
}

/// The workspace layout: a tree of leaves and splits.
export type WorkspaceNode =
  | (LeafPaneNode & { type: "leaf" })
  | (SplitPaneNode & { type: "split" });

export function leafNode(id: string): WorkspaceNode {
  return { type: "leaf", id, tabs: [], activeTabId: null };
}

/// A two-pane split between `left` and `right`, sized 50/50.
export function splitNode(
  id: string,
  direction: SplitDirection,
  left: WorkspaceNode,
  right: WorkspaceNode,
): WorkspaceNode {
  return {
    type: "split",
    id,
    direction,
    sizes: [50.0, 50.0],
    children: [left, right],
  };
}

/// All leaf panes under this node, in layout order.
export function leaves(node: WorkspaceNode): LeafPaneNode[] {
  switch (node.type) {
    case "leaf":
      return [node];
    case "split":
      return [...leaves(node.children[0]), ...leaves(node.children[1])];
  }
}

/// Find the leaf with the given id.
export function leafById(node: WorkspaceNode, id: string): LeafPaneNode | null {
  for (const leaf of leaves(node)) if (leaf.id === id) return leaf;
  return null;
}

/// The first leaf in layout order (the default target when none is active).
export function firstLeaf(node: WorkspaceNode): LeafPaneNode | null {
  return leaves(node)[0] ?? null;
}

/// Separator between a project id and a worktree path inside a workspace key.
/// Project ids are random ids that never contain it, so the first occurrence
/// always splits the two.
const WORKTREE_KEY_SEP = "::wt::";

/// The workspace key for a git worktree of `project_id`. A plain project's
/// workspace keeps using the bare project id, so saved layouts keep working;
/// only worktrees get this composite key, which gives each checkout its own
/// tabs, terminals, files and diffs while the chat still belongs to the project.
export function worktreeWorkspaceKey(
  projectId: string,
  worktreePath: string,
): string {
  return `${projectId}${WORKTREE_KEY_SEP}${worktreePath}`;
}

/// Split a workspace key into its real project id and, for a worktree
/// workspace, the worktree path.
export function splitWorkspaceKey(key: string): [string, string | null] {
  const index = key.indexOf(WORKTREE_KEY_SEP);
  if (index < 0) return [key, null];
  const path = key.slice(index + WORKTREE_KEY_SEP.length);
  return index >= 0 && path !== "" ? [key.slice(0, index), path] : [key, null];
}
