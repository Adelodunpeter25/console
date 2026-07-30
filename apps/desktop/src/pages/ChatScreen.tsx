import React from "react";
import { useAppStore, useChatStore, useProjectStore, useProviderStore } from "../store";
import { MessageList, Composer, InteractionPanel } from "../components/chat";

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
    approvalMode,
    liveToolResults,
    setInput,
    changeModel,
    setApprovalMode,
    sendMessage,
    abort,
    loadSession,
  } = useChatStore();
  const { projects } = useProjectStore();
  const { loadProviders } = useProviderStore();

  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  React.useEffect(() => {
    if (selectedSessionId) {
      loadSession(selectedSessionId);
    }
  }, [selectedSessionId, loadSession]);

  const projectName = projects.find((p) => p.id === selectedProjectId)?.name;

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
          selectedProjectId &&
          changeModel(selectedSessionId, selectedProjectId, modelId)
        }
        approvalMode={approvalMode}
        onApprovalModeChange={setApprovalMode}
        projectName={projectName}
      />
    </div>
  );
}
