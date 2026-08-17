import React from "react";
import { EmptyState } from "../components/common/EmptyState";
import { FileViewer } from "../components/file/FileViewer";
import { TerminalTab } from "../components/terminal/TerminalTab";
import { ChatScreen } from "../pages/ChatScreen";
import { api } from "../lib/api";
import { inferLanguage } from "../utils/file-language";
import { WorkspaceTabConfig, FileTabConfig, basename } from "./types";

function FileTabContent({ config }: { config: FileTabConfig }) {
  const [state, setState] = React.useState<
    { status: "loading" } | { status: "ready"; content: string } | { status: "error"; message: string }
  >({ status: "loading" });

  React.useEffect(() => {
    let cancelled = false;
    setState({ status: "loading" });
    api
      .readFile(config.path)
      .then((result) => {
        if (!cancelled) setState({ status: "ready", content: result.content });
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
    return <EmptyState title="Loading file…" description={basename(config.path)} />;
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

interface WorkspaceContentProps {
  config: WorkspaceTabConfig | null;
}

/**
 * WorkspaceContent — Active tab content renderer (Chat, FileViewer, Terminal, Diff).
 */
export function WorkspaceContent({ config }: WorkspaceContentProps) {
  if (!config) {
    return <EmptyState title="No Active Tab" description="Open a chat or select a file to display." />;
  }

  switch (config.type) {
    case "chat":
      return <ChatScreen sessionId={config.sessionId} projectId={config.projectId} />;
    case "file":
      return <FileTabContent config={config} />;
    case "terminal":
      return <TerminalTab config={config} />;
    case "diff":
      return (
        <EmptyState
          title="Diff View"
          description={config.path ? basename(config.path) : "Select a diff"}
        />
      );
  }
}
