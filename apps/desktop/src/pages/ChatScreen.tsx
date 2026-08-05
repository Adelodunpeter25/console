import React from "react";
import { useAppStore } from "../store/useAppStore";
import { useChatStore } from "../store/useChatStore";
import { useProjectStore } from "../store/useProjectStore";
import { useProviderStore } from "../store/useProviderStore";
import { useSessionStore } from "../store/useSessionStore";
import { MessageList } from "../components/chat/MessageList";
import { Composer } from "../components/chat/Composer";
import { InteractionPanel } from "../components/chat/InteractionPanel";
import { TodoList } from "../components/chat/TodoList";
import { basename } from "../utils/format";
import { EMPTY_CHAT_SESSION } from "../store/useChatStore";

/**
 * Chat view component — thin orchestrator inside pages/.
 * Rendered when a session is selected.
 */
export function ChatScreen() {
  const { selectedSessionId, selectedProjectId } = useAppStore();
  const chatSession = useChatStore((state) =>
    selectedSessionId ? state.sessions[selectedSessionId] : undefined,
  );
  const {
    messages,
    input,
    running,
    streamingText,
    streamingThinking,
    runs,
    todoItems,
    attachments,
  } = chatSession ?? EMPTY_CHAT_SESSION;
  const {
    setInput,
    sendMessage,
    abort,
    loadMessages,
    pickImages,
    addAttachments,
    removeAttachment,
  } = useChatStore();
  const {
    sessionModelId,
    sessionProvider,
    sessionCwd,
    approvalMode,
    loadSession,
    changeModel,
    changeProject,
    setApprovalMode,
  } = useSessionStore();
  const { projects, loadProjects } = useProjectStore();
  const { loadProviders, modelsByProvider } = useProviderStore();

  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  React.useEffect(() => {
    // Don't refetch if projects are already populated — re-running sets the
    // shared `loading` flag and makes the sidebar flash "Loading..." while a
    // chat is being opened.
    if (projects.length === 0) {
      loadProjects();
    }
  }, [projects.length, loadProjects]);

  React.useEffect(() => {
    if (selectedSessionId) {
      loadMessages(selectedSessionId, []);
      loadSession(selectedSessionId).then((detail) => {
        if (detail) loadMessages(selectedSessionId, detail.messages);
      });
    }
  }, [selectedSessionId, loadMessages, loadSession]);

  // Resolve the selected project: match by the session's working directory
  // first, then fall back to the app-level selection.
  const sessionProject = projects.find((p) => p.path === sessionCwd) ?? null;
  const resolvedProjectId = sessionProject?.id ?? selectedProjectId;
  const projectFallbackLabel = sessionCwd ? basename(sessionCwd) : "Select folder";
  const selectedModelSupportsImages = React.useMemo(() => {
    if (!sessionModelId || !sessionProvider) return undefined;
    return modelsByProvider[sessionProvider]?.find((model) => model.id === sessionModelId)
      ?.supportsImages;
  }, [modelsByProvider, sessionModelId, sessionProvider]);

  return (
    <div className="flex-1 flex flex-col h-full bg-screen">
      <MessageList
        messages={messages}
        streamingText={streamingText}
        streamingThinking={streamingThinking}
        running={running}
        runs={runs}
      />
      {selectedSessionId && (
        <div className="px-6 pb-1">
          <div className="max-w-3xl mx-auto">
            <InteractionPanel sessionId={selectedSessionId} />
          </div>
        </div>
      )}
      {todoItems.length > 0 && (
        <div className="px-6 pb-2">
          <div className="max-w-3xl mx-auto">
            <TodoList items={todoItems} />
          </div>
        </div>
      )}
      <Composer
        value={input}
        onChange={(value) => selectedSessionId && setInput(selectedSessionId, value)}
        onSend={() => selectedSessionId && sendMessage(selectedSessionId)}
        onAbort={() => selectedSessionId && abort(selectedSessionId)}
        running={running}
        disabled={!input.trim()}
        selectedModel={sessionModelId}
        selectedModelSupportsImages={selectedModelSupportsImages}
        onModelChange={(modelId) =>
          selectedSessionId && resolvedProjectId && changeModel(selectedSessionId, modelId)
        }
        approvalMode={approvalMode}
        onApprovalModeChange={setApprovalMode}
        projects={projects}
        selectedProjectId={resolvedProjectId}
        projectFallbackLabel={projectFallbackLabel}
        onProjectChange={(project) =>
          selectedSessionId && changeProject(selectedSessionId, project)
        }
        sessionId={selectedSessionId}
        attachments={attachments}
        messageHistory={messages
          .filter((message) => message.role === "user")
          .map((message) => message.content)}
        onPickImages={() => selectedSessionId && pickImages(selectedSessionId)}
        onAddAttachments={(items) => selectedSessionId && addAttachments(selectedSessionId, items)}
        onRemoveAttachment={(index) =>
          selectedSessionId && removeAttachment(selectedSessionId, index)
        }
      />
    </div>
  );
}
