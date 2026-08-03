import React from "react";
import { useAppStore, useChatStore, useProjectStore, useProviderStore } from "../store";
import { MessageList, Composer, InteractionPanel } from "../components/chat";
import { basename } from "../utils/format";

/**
 * Chat view component — thin orchestrator inside pages/.
 * Rendered when a session is selected.
 */
export function ChatScreen() {
  const { selectedSessionId, selectedProjectId } = useAppStore();
  const {
    messages,
    input,
    running,
    streamingText,
    streamingThinking,
    sessionModelId,
    sessionCwd,
    approvalMode,
    liveToolResults,
    setInput,
    changeModel,
    changeProject,
    setApprovalMode,
    sendMessage,
    abort,
    loadSession,
  } = useChatStore();
  const { projects, loadProjects } = useProjectStore();
  const { loadProviders } = useProviderStore();

  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  React.useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  React.useEffect(() => {
    if (selectedSessionId) {
      loadSession(selectedSessionId);
    }
  }, [selectedSessionId, loadSession]);

  // Resolve the selected project: match by the session's working directory
  // first, then fall back to the app-level selection.
  const sessionProject = projects.find((p) => p.path === sessionCwd) ?? null;
  const resolvedProjectId = sessionProject?.id ?? selectedProjectId;
  const projectFallbackLabel = sessionCwd ? basename(sessionCwd) : "Select folder";

  return (
    <div className="flex-1 flex flex-col h-full bg-screen">
      <MessageList
        messages={messages}
        streamingText={streamingText}
        streamingThinking={streamingThinking}
        running={running}
        liveToolResults={liveToolResults}
      />
      {selectedSessionId && (
        <div className="px-6 pb-1">
          <div className="max-w-3xl mx-auto">
            <InteractionPanel sessionId={selectedSessionId} />
          </div>
        </div>
      )}
      <Composer
        value={input}
        onChange={setInput}
        onSend={() => selectedSessionId && sendMessage(selectedSessionId)}
        onAbort={() => selectedSessionId && abort(selectedSessionId)}
        running={running}
        disabled={!input.trim()}
        selectedModel={sessionModelId}
        onModelChange={(modelId) =>
          selectedSessionId &&
          resolvedProjectId &&
          changeModel(selectedSessionId, resolvedProjectId, modelId)
        }
        approvalMode={approvalMode}
        onApprovalModeChange={setApprovalMode}
        projects={projects}
        selectedProjectId={resolvedProjectId}
        projectFallbackLabel={projectFallbackLabel}
        onProjectChange={(project) =>
          selectedSessionId && changeProject(selectedSessionId, project)
        }
      />
    </div>
  );
}
