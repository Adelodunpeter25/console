// The workspace panes and tabs, ported from console-ui's `workspace/`
// (`pane.rs`, `tab_bar.rs`): each leaf renders a tab strip over the active
// tab's content; each split lays its two children along its axis.
//
// Reactivity: nsh components run once, so every dynamic part re-resolves
// its node from the store's tree inside JSX (reading `revision()` there —
// the store mutates the tree in place, so a node captured in the component
// body would go stale). The desktop's scroll-follow bookkeeping
// (`TabStripFollow`) is dropped — nsh scroll views keep their own position —
// and split resizing is nsh's `ResizablePanelGroup`, whose drag-end sizes
// the store persists.

import { Text, View, token } from "nsh";
import { ResizablePanel, ResizablePanelGroup } from "nsh/base";
import { Icon } from "../common/icons";
import { SessionPane, createChatController } from "../chat/session_pane";
import {
  findNode,
  tabId,
  tabTitle,
  type LeafPaneNode,
  type WorkspaceNode,
  type WorkspaceTabConfig,
} from "./model";
import type { WorkspaceStore } from "./workspace_store";

/// The icon a tab shows, matching the desktop's tab strip.
function tabIcon(tab: WorkspaceTabConfig): Parameters<typeof Icon>[0]["name"] {
  switch (tab.type) {
    case "chat":
      return "ChatRoundLine";
    case "terminal":
      return "TerminalSquare";
    case "file":
      return "File";
    case "diff":
      return "FileDiff";
    case "browser":
      return "Globe";
    case "changesReview":
      return "ReplaceAll";
    case "settings":
      return "Settings";
  }
}

function TabStrip(props: { pane: LeafPaneNode; store: WorkspaceStore }) {
  return (
    <View class="h-9 flex-none flex-row items-center gap-0.5 border-b border-border px-1">
      {props.pane.tabs.map((tab) => {
        const active = props.pane.activeTabId === tabId(tab);
        return (
          <View
            class={
              "flex-none flex-row items-center gap-1.5 rounded-md px-2 py-1 " +
              (active ? "bg-muted" : "hover:bg-muted")
            }
            role="tab"
            accessibilityLabel={`Tab ${tabTitle(tab)}`}
            accessibilityState={{ selected: active }}
            onPress={() => props.store.selectTab(props.pane.id, tab)}
          >
            <Icon
              name={tabIcon(tab)}
              size={11}
              color={token(active ? "foreground" : "muted-foreground")}
            />
            <Text
              class={
                "max-w-[160px] truncate text-[12px] " +
                (active ? "font-medium text-foreground" : "text-muted-foreground")
              }
            >
              {tabTitle(tab)}
            </Text>
            <View
              class="rounded p-0.5 hover:bg-accent"
              role="button"
              accessibilityLabel={`Close tab ${tabTitle(tab)}`}
              onPress={() => props.store.closeTab(props.pane.id, tab)}
            >
              <Icon name="X" size={11} color={token("muted-foreground")} />
            </View>
          </View>
        );
      })}
    </View>
  );
}

/// One tab's content. Chat tabs render the real transcript through the
/// native bridge; a chat tab without a real session (the welcome tab,
/// `sessionId: "welcome"`) keeps the headless-safe placeholder. Controllers
/// live in a module-level cache keyed by session id, so switching tabs back
/// keeps the loaded transcript instead of reloading (the desktop keeps one
/// transcript entity per session the same way).
const chatControllers = new Map<string, ReturnType<typeof createChatController>>();

/// The controller for one session's chat tab, creating it on first use. The
/// launcher (App.tsx) calls this too, so its run streams into the same
/// transcript the pane renders.
export function chatController(baseUrl: string, sessionId: string) {
  let controller = chatControllers.get(sessionId);
  if (controller == null) {
    controller = createChatController(baseUrl, sessionId);
    chatControllers.set(sessionId, controller);
  }
  return controller;
}

function PaneContent(props: { pane: LeafPaneNode; baseUrl: string }) {
  return (
    <View class="flex-1 flex-col">
      {(() => {
        const tab = props.pane.tabs.find(
          (t) => tabId(t) === props.pane.activeTabId,
        );
        if (tab == null) {
          return (
            <View class="flex-1 items-center justify-center">
              <Text class="text-muted-foreground">No tab open</Text>
            </View>
          );
        }
        if (tab.type === "chat" && tab.sessionId !== "welcome") {
          return (
            <SessionPane controller={chatController(props.baseUrl, tab.sessionId)} />
          );
        }
        return (
          <View class="flex-1 items-center justify-center">
            <Text class="text-muted-foreground">{tabTitle(tab)}</Text>
          </View>
        );
      })()}
    </View>
  );
}

/// Renders the node with `nodeId`, re-resolved from the tree inside JSX:
/// when a pane closes, its node leaves the tree and this renders nothing.
function NodeView(props: { nodeId: string; store: WorkspaceStore; baseUrl: string }) {
  return (
    <View class="flex-1 flex-col overflow-hidden">
      {(() => {
        // The revision read makes this IIFE re-run on every tree edit.
        props.store.revision();
        const node = findNode(props.store.tree(), props.nodeId);
        if (node == null) return null;
        if (node.type === "leaf") {
          return (
            <View
              class="flex-1 flex-col"
              accessibilityLabel={`Pane ${node.id}`}
              onPress={() => props.store.focusPane(node.id)}
            >
              <TabStrip pane={node} store={props.store} />
              <PaneContent pane={node} baseUrl={props.baseUrl} />
            </View>
          );
        }
        return (
          <ResizablePanelGroup
            axis={node.direction === "horizontal" ? "horizontal" : "vertical"}
            onResize={({ sizes: next }) => {
              props.store.resizeSplit(node.id, next[0] ?? 50);
            }}
            class="flex-1"
          >
            <ResizablePanel size={node.sizes[0]} minSize={10}>
              <NodeView nodeId={node.children[0]!.id} store={props.store} baseUrl={props.baseUrl} />
            </ResizablePanel>
            <ResizablePanel size={node.sizes[1]} minSize={10}>
              <NodeView nodeId={node.children[1]!.id} store={props.store} baseUrl={props.baseUrl} />
            </ResizablePanel>
          </ResizablePanelGroup>
        );
      })()}
    </View>
  );
}

/// The whole workspace: every pane, split and tab, driven by the store.
export function WorkspaceView(props: { store: WorkspaceStore; baseUrl?: string }) {
  return (
    <View class="flex-1 flex-col overflow-hidden">
      {(() => {
        const baseUrl = props.baseUrl ?? "";
        props.store.revision();
        return <NodeView nodeId={props.store.tree().id} store={props.store} baseUrl={baseUrl} />;
      })()}
    </View>
  );
}
