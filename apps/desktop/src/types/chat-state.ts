import type { ChatSessionState } from "./chat";

export function createChatSessionState(): ChatSessionState {
  return {
    messages: [],
    input: "",
    running: false,
    streamingText: "",
    streamingThinking: "",
    pendingQuestion: null,
    pendingPermissions: [],
    liveToolResults: [],
    activeToolCalls: [],
    todoItems: [],
    runActivity: { startedAt: null, elapsedMs: 0, calls: [], results: [] },
    attachments: [],
  };
}

export const EMPTY_CHAT_SESSION = createChatSessionState();

export function getChatSessionState(
  sessions: Record<string, ChatSessionState>,
  sessionId: string,
): ChatSessionState {
  return sessions[sessionId] ?? createChatSessionState();
}

export function updateChatSession(
  sessions: Record<string, ChatSessionState>,
  sessionId: string,
  update: (state: ChatSessionState) => ChatSessionState,
): Record<string, ChatSessionState> {
  return {
    ...sessions,
    [sessionId]: update(getChatSessionState(sessions, sessionId)),
  };
}
