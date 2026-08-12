import React from "react";
import { Columns, Rows, Plus, X } from "lucide-react";
import { LeafPaneNode, WorkspaceTabConfig, getTabId } from "./types";
import { WorkspaceTabItem } from "./WorkspaceTabItem";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { useProjectStore } from "../store/useProjectStore";

interface WorkspaceTabBarProps {
  pane: LeafPaneNode;
  canClosePane: boolean;
}

/**
 * WorkspaceTabBar — Top actions & tab bar header for each split pane tile.
 * Includes split controls (Split Right, Split Down) and pane management.
 */
export function WorkspaceTabBar({ pane, canClosePane }: WorkspaceTabBarProps) {
  const setActiveTab = useWorkspaceStore((state) => state.setActiveTab);
  const closeTab = useWorkspaceStore((state) => state.closeTab);
  const splitPane = useWorkspaceStore((state) => state.splitPane);
  const closePane = useWorkspaceStore((state) => state.closePane);
  const createSession = useProjectStore((state) => state.createSession);
  const openChatTab = useWorkspaceStore((state) => state.openChatTab);

  const handleNewTab = async () => {
    const session = await createSession("", "", "New Chat");
    openChatTab({
      type: "chat",
      projectId: "",
      sessionId: session.id,
      title: session.title,
    });
  };

  return (
    <div className="flex items-center justify-between h-9 bg-sidebar border-b border-border shrink-0 select-none overflow-hidden">
      {/* Scrollable Tabs List */}
      <div className="flex items-center h-full overflow-x-auto overflow-y-hidden flex-1 no-scrollbar">
        {pane.tabs.map((tab) => {
          const tabId = getTabId(tab);
          const isActive = pane.activeTabId === tabId;
          return (
            <WorkspaceTabItem
              key={tabId}
              config={tab}
              isActive={isActive}
              onSelect={() => setActiveTab(pane.id, tabId)}
              onClose={() => closeTab(pane.id, tabId)}
            />
          );
        })}
        <button
          onClick={handleNewTab}
          className="p-1.5 ml-1 rounded text-foreground-muted hover:text-foreground hover:bg-white/[0.06] transition-colors cursor-pointer"
          title="New Chat Tab"
          aria-label="New Chat Tab"
        >
          <Plus size={14} />
        </button>
      </div>

      {/* Split Pane Control Actions */}
      <div className="flex items-center gap-1 px-2 shrink-0 border-l border-border bg-sidebar">
        <button
          onClick={() => splitPane(pane.id, "horizontal")}
          className="p-1.5 rounded text-foreground-muted hover:text-foreground hover:bg-white/[0.06] transition-colors cursor-pointer"
          title="Split Pane Right (Left/Right)"
          aria-label="Split Right"
        >
          <Columns size={14} />
        </button>
        <button
          onClick={() => splitPane(pane.id, "vertical")}
          className="p-1.5 rounded text-foreground-muted hover:text-foreground hover:bg-white/[0.06] transition-colors cursor-pointer"
          title="Split Pane Down (Top/Bottom)"
          aria-label="Split Down"
        >
          <Rows size={14} />
        </button>
        {canClosePane && (
          <button
            onClick={() => closePane(pane.id)}
            className="p-1.5 rounded text-foreground-muted hover:text-danger hover:bg-white/[0.06] transition-colors cursor-pointer"
            title="Close Split Tile"
            aria-label="Close Tile"
          >
            <X size={14} />
          </button>
        )}
      </div>
    </div>
  );
}
