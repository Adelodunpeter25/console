import React from "react";
import { Layout, TabNode } from "flexlayout-react";
import type { ITabRenderValues } from "flexlayout-react";
import { EmptyState } from "../components/common/EmptyState";
import { FileViewer } from "../components/file/FileViewer";
import { TerminalTab } from "../components/terminal/TerminalTab";
import { ChatScreen } from "../pages/ChatScreen";
import { tauriApi } from "../lib/tauri-api";
import { basename } from "./types";
import { useAppStore } from "../store/useAppStore";
import { getActiveWorkspaceTab } from "./model";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { isWorkspaceTabConfig } from "./types";
import type { FileTabConfig, WorkspaceTabConfig } from "./types";
import { WorkspaceTabItem } from "./WorkspaceTabItem";
import { inferLanguage } from "../utils/file-language";

function FileTab({ config }: { config: FileTabConfig }) {
  const [state, setState] = React.useState<
    { status: "loading" } | { status: "ready"; content: string } | { status: "error"; message: string }
  >({ status: "loading" });

  React.useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });
    tauriApi
      .readFile(config.path)
      .then((result) => {
        if (!cancelled) {
          setState({ status: "ready", content: result.content });
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          const message = err instanceof Error ? err.message : String(err);
          setState({ status: "error", message });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [config.path]);

  if (state.status === "loading") {
    return (
      <EmptyState title="Loading file…" description={basename(config.path)} />
    );
  }

  if (state.status === "error") {
    return (
      <EmptyState
        title="Could not open file"
        description={`${basename(config.path)} — ${state.message}`}
      />
    );
  }

  return (
    <FileViewer
      content={state.content}
      fileName={basename(config.path)}
      language={inferLanguage(config.path)}
    />
  );
}

function WorkspaceTab({ config }: { config: WorkspaceTabConfig }) {
  switch (config.type) {
    case "chat":
      return <ChatScreen sessionId={config.sessionId} projectId={config.projectId} />;
    case "file":
      return <FileTab config={config} />;
    case "terminal":
      return <TerminalTab config={config} />;
    case "diff":
      return (
        <EmptyState
          title="Diff view"
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