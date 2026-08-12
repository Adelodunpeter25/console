import React from "react";
import { X } from "lucide-react";
import { LeafPaneNode, getTabId } from "./types";
import { WorkspaceTabItem } from "./WorkspaceTabItem";
import { useWorkspaceStore } from "./useWorkspaceStore";

interface WorkspaceTabBarProps {
  pane: LeafPaneNode;
  canClosePane: boolean;
}

/**
 * WorkspaceTabBar — Clean header bar for workspace tabs without extra header clutter or gaps.
 */
export function WorkspaceTabBar({ pane, canClosePane }: WorkspaceTabBarProps) {
  const setActiveTab = useWorkspaceStore((state) => state.setActiveTab);
  const closeTab = useWorkspaceStore((state) => state.closeTab);
  const closePane = useWorkspaceStore((state) => state.closePane);

  return (
    <div className="flex items-center justify-between h-9 bg-black border-b border-border shrink-0 select-none overflow-hidden">
      {/* Scrollable Tabs List */}
      <div className="flex items-center h-full overflow-x-auto overflow-y-hidden flex-1 no-scrollbar">
        {pane.tabs.map((tab) => {
          const tabId = getTabId(tab);
          const isActive = pane.activeTabId === tabId;
          return (
            <WorkspaceTabItem
              key={tabId}
              paneId={pane.id}
              config={tab}
              isActive={isActive}
              onSelect={() => setActiveTab(pane.id, tabId)}
              onClose={() => closeTab(pane.id, tabId)}
            />
          );
        })}
      </div>

      {/* Pane Close Action (If multiple split tiles exist) */}
      {canClosePane && (
        <div className="flex items-center px-2 shrink-0 border-l border-border bg-black">
          <button
            onClick={() => closePane(pane.id)}
            className="p-1.5 rounded-none text-foreground-muted hover:text-danger hover:bg-white/[0.06] transition-colors cursor-pointer"
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
