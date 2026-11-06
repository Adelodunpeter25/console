// Tests for the pane-tree ops, pinning the desktop's semantics
// (`console-ui/src/workspace/ops.rs`, exercised by its
// `tests/workspace/workspace_test.rs`): neighbor activation on close,
// sibling collapse on pane close, dedupe-on-open, split injection.
import { expect, test } from "nsh/test";
import { leafNode, leaves, splitNode, tabId, type WorkspaceNode } from "./model";
import {
  activeLeaf,
  closePane,
  closeTab,
  findSplitSizes,
  moveTabToPane,
  moveTabToSplit,
  openTab,
  resizeSplit,
  selectTab,
  splitPane,
} from "./ops";

const chat = (sessionId: string, title = sessionId) =>
  ({ type: "chat", sessionId, title }) as const;

function twoPaneRoot(): WorkspaceNode {
  return splitNode("split-1", "horizontal", leafNode("pane-a"), leafNode("pane-b"));
}

test("open_tab adds and activates, and dedupes", () => {
  const root = leafNode("pane-main");
  openTab(root, "pane-main", chat("s1"));
  openTab(root, "pane-main", chat("s1", "renamed"));
  const leaf = root.type === "leaf" ? root : null;
  expect(leaf!.tabs.length).toBe(1);
  expect(leaf!.activeTabId).toBe("chat:s1");
});

test("active_leaf falls back to the first leaf, or the requested pane", () => {
  const root = twoPaneRoot();
  expect(activeLeaf(root, null)!.id).toBe("pane-a");
  expect(activeLeaf(root, "nope")!.id).toBe("pane-a");
  expect(activeLeaf(root, "pane-b")!.id).toBe("pane-b");
});

test("close_tab activates the left neighbor, else the first tab", () => {
  const root = leafNode("pane-main");
  openTab(root, "pane-main", chat("s1"));
  openTab(root, "pane-main", chat("s2"));
  openTab(root, "pane-main", chat("s3"));
  selectTab(root, "pane-main", "chat:s2");
  expect(closeTab(root, "pane-main", "chat:s2")).toBe("chat:s1");
  expect(closeTab(root, "pane-main", "chat:s1")).toBe("chat:s3");
});

test("close_pane collapses the parent split into the sibling and keeps the last pane", () => {
  const root = twoPaneRoot();
  openTab(root, "pane-a", chat("s1"));
  openTab(root, "pane-b", chat("s2"));
  expect(closePane(root, "pane-b")).toBe(true);
  expect(root.type).toBe("leaf");
  expect((root as any).id).toBe("pane-a");
  // The final pane is never removed.
  expect(closePane(root, "pane-a")).toBe(false);
});

test("split_pane injects a fresh empty pane beside the original", () => {
  const root = leafNode("pane-a");
  openTab(root, "pane-a", chat("s1"));
  const newId = splitPane(root, "pane-a", "vertical");
  expect(newId).toBeTruthy();
  const [a, b] = root.type === "split" ? root.children : [];
  expect(a!.type).toBe("leaf");
  expect((a as any).tabs.length).toBe(1);
  expect(b!.type).toBe("leaf");
  expect((b as any).tabs.length).toBe(0);
});

test("move_tab_to_pane dedupes and activates instead of duplicating", () => {
  const root = twoPaneRoot();
  openTab(root, "pane-a", chat("s1"));
  openTab(root, "pane-b", chat("s2"));
  expect(moveTabToPane(root, "pane-a", "pane-b", chat("s1"))).toBe("pane-b");
  const b = root.type === "split" ? root.children[1] : null;
  expect(b != null && b.type === "leaf" && b.tabs.map((t) => tabId(t))).toEqual(["chat:s2", "chat:s1"]);
  expect(b != null && b.type === "leaf" && b.activeTabId).toBe("chat:s1");
});

test("move_tab_to_split detaches the tab into a fresh adjacent pane", () => {
  const root = twoPaneRoot();
  openTab(root, "pane-a", chat("s1"));
  const newPane = moveTabToSplit(root, "pane-a", "pane-b", chat("s1"), "horizontal", true);
  expect(newPane).toBeTruthy();
  // insert_first: the new pane sits left of the target, which itself keeps
  // its position in the tree (layout order: pane-a, new pane, pane-b).
  const order = leaves(root).map((l) => l.id);
  expect(order).toEqual(["pane-a", newPane, "pane-b"]);
});

test("resize_split clamps and finds sizes", () => {
  const root = twoPaneRoot();
  expect(resizeSplit(root, "split-1", 5)).toBe(true);
  expect(findSplitSizes(root, "split-1")).toEqual([10, 90]);
  expect(resizeSplit(root, "split-1", 95)).toBe(true);
  expect(findSplitSizes(root, "split-1")).toEqual([90, 10]);
  expect(resizeSplit(root, "nope", 50)).toBe(false);
});
