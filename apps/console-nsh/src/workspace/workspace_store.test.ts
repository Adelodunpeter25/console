// Tests for the workspace store: reactivity (each mutation bumps the
// revision) and the bookkeeping around the pure ops (close-tab-closes-empty-
// pane, active pane cursor).
import { expect, test } from "nsh/test";
import { splitNode, type WorkspaceNode } from "./model";
import { createWorkspaceStore } from "./workspace_store";

const chat = (sessionId: string) =>
  ({ type: "chat" as const, sessionId, title: sessionId });

test("open/select/close bookkeeping and revision bumps", () => {
  const store = createWorkspaceStore();
  const rev0 = store.revision();

  store.openTab("pane-main", chat("s1"));
  expect(store.revision()).toBe(rev0 + 1);
  expect(store.activePaneId()).toBe("pane-main");

  store.selectTab("pane-main", chat("s1"));
  expect(store.activePaneId()).toBe("pane-main");
});

test("closing a tab keeps the pane; closing a pane's last tab closes the pane", () => {
  const root: WorkspaceNode = splitNode(
    "split-1",
    "horizontal",
    { type: "leaf", id: "pane-a", tabs: [], activeTabId: null },
    { type: "leaf", id: "pane-b", tabs: [], activeTabId: null },
  );
  const store = createWorkspaceStore(root);
  store.openTab("pane-a", chat("s1"));
  store.openTab("pane-b", chat("s2"));
  store.openTab("pane-b", chat("s3"));

  // pane-b keeps a tab: the pane stays.
  const stillB = store.closeTab("pane-b", chat("s3"));
  expect(stillB).toBe("pane-b");
  expect(store.panes().length).toBe(2);

  // pane-a's last tab: the pane goes, the sibling collapses into the root.
  store.focusPane("pane-b");
  const after = store.closeTab("pane-a", chat("s1"));
  expect(store.panes().length).toBe(1);
  expect(store.panes()[0]!.id).toBe("pane-b");
  expect(after).toBe("pane-b");
});
