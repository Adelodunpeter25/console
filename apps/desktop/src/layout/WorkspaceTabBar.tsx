import React from "react";
import { Plus, X } from "lucide-react";
import { LeafPaneNode, getTabId } from "./types";
import { WorkspaceTabItem } from "./WorkspaceTabItem";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { useProjectStore } from "../store/useProjectStore";

interface WorkspaceTabBarProps {
  pane: LeafPaneNode;
  canClosePane: boolean;
}

/**
 * WorkspaceTabBar — Top actions & tab bar header for each split pane tile.
 * Clean, lightweight tab bar without header split clutter.
 */
export function WorkspaceTabBar({ pane, canClosePane }: WorkspaceTabBarProps) {
  const setActiveTab = useWorkspaceStore((state) => state.setActiveTab);
  const closeTab = useWorkspaceStore((state) => state.closeTab);
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

      {/* Pane Close Action (If multiple split tiles exist) */}
      {canClosePane && (
        <div className="flex items-center px-2 shrink-0 border-l border-border bg-sidebar">
          <button
            onClick={() => closePane(pane.id)}
            className="p-1.5 rounded text-foreground-muted hover:text-danger hover:bg-white/[0.06] transition-colors cursor-pointer"
            title="Close Split Tile"
            aria-label="Close Tile"
          >
            <X size={14} />
          </button>
        </div>
      )}
    </div>
  );
}
