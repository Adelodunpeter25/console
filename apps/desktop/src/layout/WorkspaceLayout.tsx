import React from "react";
import { Layout, TabNode } from "flexlayout-react";
import type { ITabRenderValues } from "flexlayout-react";
import { EmptyState } from "../components/common/EmptyState";
import { ChatScreen } from "../pages/ChatScreen";
import { useAppStore } from "../store/useAppStore";
import { getActiveWorkspaceTab } from "./model";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { isWorkspaceTabConfig } from "./types";
import type { WorkspaceTabConfig } from "./types";
import { WorkspaceTabItem } from "./WorkspaceTabItem";

function WorkspaceTab({ config }: { config: WorkspaceTabConfig }) {
  switch (config.type) {
    case "chat":
      return <ChatScreen sessionId={config.sessionId} projectId={config.projectId} />;
    case "terminal":
    case "file":
    case "diff":
      return (
        <EmptyState
          title={`${config.type[0]!.toUpperCase()}${config.type.slice(1)} view`}
          description="This workspace tab is reserved for a future view."
        />
      );
  }
}

export function WorkspaceLayout() {
  const model = useWorkspaceStore((state) => state.model);
  const workspaceRevision = useWorkspaceStore((state) => state.revision);
  const notifyLayoutChange = useWorkspaceStore((state) => state.notifyLayoutChange);
  const setSelectedProjectId = useAppStore((state) => state.setSelectedProjectId);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);

  const syncActiveTab = React.useCallback(
    (nextModel: typeof model) => {
      const activeTab = getActiveWorkspaceTab(nextModel);
      if (activeTab?.type === "chat") {
        setSelectedProjectId(activeTab.projectId);
        setSelectedSessionId(activeTab.sessionId);
      } else {
        setSelectedProjectId(activeTab?.projectId ?? null);
        setSelectedSessionId(null);
      }
    },
    [setSelectedProjectId, setSelectedSessionId],
  );

  const handleModelChange = React.useCallback(
    (nextModel: typeof model) => {
      notifyLayoutChange();
      syncActiveTab(nextModel);
    },
    [notifyLayoutChange, syncActiveTab],
  );

  React.useEffect(() => {
    syncActiveTab(model);
  }, [model, syncActiveTab]);

  const factory = React.useCallback((node: TabNode) => {
    const config = node.getConfig();
    return isWorkspaceTabConfig(config) ? <WorkspaceTab config={config} /> : null;
  }, []);

  const renderTab = React.useCallback((node: TabNode, renderValues: ITabRenderValues) => {
    renderValues.content = <WorkspaceTabItem node={node} />;
  }, []);

  const hasTabs = React.useMemo(() => {
    let foundTab = false;
    model.visitNodes((node) => {
      if (node instanceof TabNode) foundTab = true;
    });
    return foundTab;
  }, [model, workspaceRevision]);

  return (
    <div
      className={`workspace-layout relative h-full w-full min-h-0${
        hasTabs ? "" : " workspace-layout--empty"
      }`}
    >
      <Layout
        model={model}
        factory={factory}
        onRenderTab={renderTab}
        onModelChange={handleModelChange}
        onTabSetPlaceHolder={() => (
          <EmptyState
            title="No Session Selected"
            description="Select or create a chat session from the sidebar."
          />
        )}
      />
    </div>
  );
}
