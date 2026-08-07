import React from "react";
import { Layout, TabNode } from "flexlayout-react";
import type { ITabRenderValues } from "flexlayout-react";
import { EmptyState } from "../components/common/EmptyState";
import { FileViewer } from "../components/file/FileViewer";
import { ChatScreen } from "../pages/ChatScreen";
import { tauriApi } from "../lib/tauri-api";
import { basename } from "./types";
import { useAppStore } from "../store/useAppStore";
import { getActiveWorkspaceTab } from "./model";
import { useWorkspaceStore } from "./useWorkspaceStore";
import { isWorkspaceTabConfig } from "./types";
import type { FileTabConfig, WorkspaceTabConfig } from "./types";
import { WorkspaceTabItem } from "./WorkspaceTabItem";

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

function inferLanguage(filePath: string): string | undefined {
  const ext = filePath.slice(filePath.lastIndexOf(".") + 1).toLowerCase();
  const map: Record<string, string> = {
    ts: "typescript",
    tsx: "tsx",
    js: "javascript",
    jsx: "jsx",
    mjs: "javascript",
    cjs: "javascript",
    json: "json",
    md: "markdown",
    py: "python",
    rs: "rust",
    go: "go",
    rb: "ruby",
    java: "java",
    kt: "kotlin",
    swift: "swift",
    cs: "csharp",
    cpp: "cpp",
    cc: "cpp",
    cxx: "cpp",
    c: "c",
    h: "c",
    hpp: "cpp",
    html: "html",
    css: "css",
    scss: "scss",
    sass: "sass",
    less: "less",
    yaml: "yaml",
    yml: "yaml",
    toml: "toml",
    sh: "bash",
    bash: "bash",
    zsh: "bash",
    sql: "sql",
    php: "php",
    vue: "vue",
    svelte: "svelte",
    lua: "lua",
    r: "r",
    dockerfile: "dockerfile",
  };
  if (map[ext]) return map[ext];
  const base = basename(filePath).toLowerCase();
  if (base === "dockerfile") return "dockerfile";
  return undefined;
}

function WorkspaceTab({ config }: { config: WorkspaceTabConfig }) {
  switch (config.type) {
    case "chat":
      return <ChatScreen sessionId={config.sessionId} projectId={config.projectId} />;
    case "file":
      return <FileTab config={config} />;
    case "terminal":
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
