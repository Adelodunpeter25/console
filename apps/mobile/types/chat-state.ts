import type { ChatSessionState, ChatSnapshot } from "./chat";

export function createChatSessionState(): ChatSessionState {
  return {
    messages: [],
    input: "",
    running: false,
    streamingText: "",
    streamingThinking: "",
    pendingQuestions: [],
    pendingPermissions: [],
    activeToolCalls: [],
    todoItems: [],
    subagents: [],
    runs: [],
    attachments: [],
    draftUpdatedAt: undefined,
  };
}

export const EMPTY_CHAT_SESSION = createChatSessionState();

export const emptyChatSnapshot: ChatSnapshot = {
  messages: [],
  streamingText: "",
  streamingThinking: "",
  activeToolCalls: [],
  liveToolResults: [],
  pendingPermission: null,
  pendingQuestion: null,
  pendingPermissions: [],
  pendingQuestions: [],
  todoItems: [],
  subagents: [],
  runs: [],
  running: false,
};

export function getChatSessionState(
  sessions: Record<string, ChatSessionState>,
  sessionId: string,
): ChatSessionState {
  return sessions[sessionId] ?? EMPTY_CHAT_SESSION;
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
