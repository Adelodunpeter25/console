import React from "react";
import { useAppStore, useChatStore, useProjectStore, useProviderStore } from "../store";
import { MessageList, Composer } from "../components/chat";

/**
 * Chat screen — thin orchestrator.
 *
 * Rendered only when a session is selected (ChatPage handles the empty state).
 * All business logic (model resolution, persistence, send/abort) lives in
 * the stores. This component only wires stores to UI components.
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
    setInput,
    changeModel,
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
      />
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
        projectName={projectName}
      />
    </div>
  );
}
