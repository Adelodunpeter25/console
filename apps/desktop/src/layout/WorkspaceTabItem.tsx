import type { TabNode } from "flexlayout-react";
import { isWorkspaceTabConfig } from "./types";
import { useProjectStore } from "../store/useProjectStore";

interface WorkspaceTabItemProps {
  node: TabNode;
}

export function WorkspaceTabItem({ node }: WorkspaceTabItemProps) {
  const config = node.getConfig();
  const sessionId =
    isWorkspaceTabConfig(config) && config.type === "chat" ? config.sessionId : null;
  const sessionTitle = useProjectStore((state) =>
    sessionId ? state.sessions.find((session) => session.id === sessionId)?.title : undefined,
  );
  const title = sessionTitle || node.getName() || "Untitled Chat";

  return (
    <span className="workspace-tab-item" title={title}>
      {title}
    </span>
  );
}
