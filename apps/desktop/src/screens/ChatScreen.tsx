import React from "react";
import { useAppStore, useChatStore, useProjectStore, useProviderStore } from "../store";
import { MessageList, Composer } from "../components/chat";

/**
 * Chat screen — thin orchestrator.
 *
 * Rendered only when a session is selected (ChatPage handles the empty state).
 * Owns store wiring, session lifecycle, the selected-model state, and the
 * send/abort handlers. All rendering is delegated to MessageList + Composer.
 */
export function ChatScreen() {
  const { selectedSessionId, selectedProjectId } = useAppStore();
  const {
    messages,
    input,
    running,
    streamingText,
    streamingThinking,
    setInput,
    sendMessage,
    abort,
    loadSession,
  } = useChatStore();
  const { projects } = useProjectStore();
  const { loadProviders } = useProviderStore();

  const [selectedModel, setSelectedModel] = React.useState<string | null>(null);

  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  React.useEffect(() => {
    if (selectedSessionId) {
      loadSession(selectedSessionId);
    }
  }, [selectedSessionId, loadSession]);

  const handleSend = React.useCallback(() => {
    if (!selectedSessionId || !input.trim() || running) return;
    void sendMessage(selectedSessionId);
  }, [selectedSessionId, input, running, sendMessage]);

  const handleAbort = React.useCallback(() => {
    if (selectedSessionId) void abort(selectedSessionId);
  }, [selectedSessionId, abort]);

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
        onSend={handleSend}
        onAbort={handleAbort}
        running={running}
        disabled={!input.trim()}
        selectedModel={selectedModel}
        onModelChange={setSelectedModel}
        projectName={projectName}
      />
    </div>
  );
}
