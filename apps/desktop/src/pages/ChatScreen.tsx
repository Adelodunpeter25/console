import React from "react";
import { toast } from "sonner";
import type { ApprovalMode, ImageAttachment, ProviderId } from "@console/types";
import { useChatStore } from "../store/useChatStore";
import { useProjectStore } from "../store/useProjectStore";
import { useProviderStore } from "../store/useProviderStore";
import { EMPTY_SESSION_VIEW, useSessionStore } from "../store/useSessionStore";
import { useWorkspaceStore } from "../layout/useWorkspaceStore";
import { MessageList } from "../components/chat/MessageList";
import { Composer } from "../components/chat/Composer";
import { InteractionPanel } from "../components/chat/InteractionPanel";
import { TodoList } from "../components/chat/TodoList";
import { basename } from "../utils/format";
import { EMPTY_CHAT_SESSION } from "../store/useChatStore";
import { useShallow } from "zustand/react/shallow";

/**
 * Chat view component — thin orchestrator inside pages/.
 * Rendered when a session is selected.
 */
interface ChatScreenProps {
  sessionId: string;
  projectId: string;
}

export function ChatScreen({ sessionId, projectId }: ChatScreenProps) {
  const selectedSessionId = sessionId;
  const selectedProjectId = projectId;
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
  const setInput = useChatStore((state) => state.setInput);
  const sendMessage = useChatStore((state) => state.sendMessage);
  const abort = useChatStore((state) => state.abort);
  const loadMessages = useChatStore((state) => state.loadMessages);
  const pickImages = useChatStore((state) => state.pickImages);
  const addAttachments = useChatStore((state) => state.addAttachments);
  const removeAttachment = useChatStore((state) => state.removeAttachment);
  const { sessionModelId, sessionProvider, sessionCwd, approvalMode } = useSessionStore(
    (state) => state.sessions[sessionId] ?? EMPTY_SESSION_VIEW,
  );
  const { loadSession, changeModel, changeProject, setApprovalMode } = useSessionStore(
    useShallow((state) => ({
      loadSession: state.loadSession,
      changeModel: state.changeModel,
      changeProject: state.changeProject,
      setApprovalMode: state.setApprovalMode,
    })),
  );
  const updateChatTabProject = useWorkspaceStore((state) => state.updateChatTabProject);
  const projects = useProjectStore((state) => state.projects);
  const loadProjects = useProjectStore((state) => state.loadProjects);
  const loadProviders = useProviderStore((state) => state.loadProviders);
  const modelsByProvider = useProviderStore((state) => state.modelsByProvider);

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

  const handleProjectChange = (project: (typeof projects)[number]) => {
    if (!selectedSessionId) return;
    changeProject(selectedSessionId, project);
    updateChatTabProject(selectedSessionId, project.id);
  };

  const messageHistory = React.useMemo(
    () => messages.filter((message) => message.role === "user").map((message) => message.content),
    [messages],
  );

  const handleInputChange = React.useCallback(
    (value: string) => {
      if (selectedSessionId) setInput(selectedSessionId, value);
    },
    [selectedSessionId, setInput],
  );

  const handleSend = React.useCallback(() => {
    if (!selectedSessionId) return;
    // A working folder must be chosen before the first message — the agent
    // operates on the selected project's files. Prompt instead of silently
    // running in the server's cwd.
    if (!resolvedProjectId) {
      toast.message("Select a folder first — use the folder picker below the composer.");
      return;
    }
    sendMessage(selectedSessionId);
  }, [selectedSessionId, resolvedProjectId, sendMessage]);

  const handleAbort = React.useCallback(() => {
    if (selectedSessionId) abort(selectedSessionId);
  }, [selectedSessionId, abort]);

  const handleModelChange = React.useCallback(
    (modelId: string, provider?: ProviderId) => {
      if (selectedSessionId && resolvedProjectId) changeModel(selectedSessionId, modelId, provider);
    },
    [selectedSessionId, resolvedProjectId, changeModel],
  );

  const handleApprovalModeChange = React.useCallback(
    (mode: ApprovalMode) => {
      setApprovalMode(sessionId, mode);
    },
    [sessionId, setApprovalMode],
  );

  const handlePickImages = React.useCallback(() => {
    if (selectedSessionId) pickImages(selectedSessionId);
  }, [selectedSessionId, pickImages]);

  const handleAddAttachments = React.useCallback(
    (items: ImageAttachment[]) => {
      if (selectedSessionId) addAttachments(selectedSessionId, items);
    },
    [selectedSessionId, addAttachments],
  );

  const handleRemoveAttachment = React.useCallback(
    (index: number) => {
      if (selectedSessionId) removeAttachment(selectedSessionId, index);
    },
    [selectedSessionId, removeAttachment],
  );

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
        onChange={handleInputChange}
        onSend={handleSend}
        onAbort={handleAbort}
        running={running}
        disabled={!input.trim() || !resolvedProjectId}
        selectedModel={sessionModelId}
        selectedProvider={sessionProvider}
        selectedModelSupportsImages={selectedModelSupportsImages}
        onModelChange={handleModelChange}
        approvalMode={approvalMode}
        onApprovalModeChange={handleApprovalModeChange}
        projects={projects}
        selectedProjectId={resolvedProjectId}
        projectFallbackLabel={projectFallbackLabel}
        onProjectChange={handleProjectChange}
        sessionId={selectedSessionId}
        attachments={attachments}
        messageHistory={messageHistory}
        onPickImages={handlePickImages}
        onAddAttachments={handleAddAttachments}
        onRemoveAttachment={handleRemoveAttachment}
      />
    </div>
  );
}
