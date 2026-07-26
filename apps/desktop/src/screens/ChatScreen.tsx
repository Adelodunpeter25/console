import React from "react";
import { Brain } from "lucide-react";
import { useAppStore, useChatStore, useProjectStore, useProviderStore } from "../store";
import { EmptyState } from "../components/common";
import { MessageList, Composer } from "../components/chat";

/**
 * Chat screen — thin orchestrator.
 *
 * Owns only store wiring, session lifecycle, the selected-model state, and
 * the send/abort handlers. All rendering is delegated:
 *   - MessageList  → memoized message rows + streaming bubble + scroll logic
 *   - Composer     → textarea, model selector, send/stop toolbar
 *   - EmptyState   → "No Session Selected" placeholder
 *
 * Keeping this screen small means a streaming token updates the chat store,
 * which flows into MessageList/StreamingBubble only — the screen itself does
 * not re-render the message bubbles. (Conductor rewrite lesson.)
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
    clear,
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
    } else {
      clear();
    }
  }, [selectedSessionId, loadSession, clear]);

  const handleSend = React.useCallback(() => {
    if (!selectedSessionId || !input.trim() || running) return;
    void sendMessage(selectedSessionId);
  }, [selectedSessionId, input, running, sendMessage]);

  const handleAbort = React.useCallback(() => {
    if (selectedSessionId) void abort(selectedSessionId);
  }, [selectedSessionId, abort]);

  const projectName = projects.find((p) => p.id === selectedProjectId)?.name;

  if (!selectedSessionId) {
    return (
      <EmptyState
        icon={Brain}
        title="No Session Selected"
        description="Select or create a chat session from the sidebar to start chatting with the agent."
      />
    );
  }

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
