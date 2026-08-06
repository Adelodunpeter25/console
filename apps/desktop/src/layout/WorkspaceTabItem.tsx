import type { TabNode } from "flexlayout-react";

interface WorkspaceTabItemProps {
  node: TabNode;
}

export function WorkspaceTabItem({ node }: WorkspaceTabItemProps) {
  const title = node.getName();

  return (
    <span className="workspace-tab-item" title={title}>
      {title}
    </span>
  );
}
