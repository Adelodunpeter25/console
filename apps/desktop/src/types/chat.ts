import type {
  AgentMessage,
  AgentSessionEvent,
  AskQuestionRequest,
  ImageAttachment,
  PermissionRequest,
  ToolCall,
  ToolResult,
  TodoItem,
} from "@console/types";

export interface PendingQuestion {
  request: AskQuestionRequest;
}

export interface PendingPermission {
  request: PermissionRequest;
}

export interface RunActivityState {
  startedAt: number | null;
  elapsedMs: number;
  calls: ToolCall[];
  results: ToolResult[];
}

export interface ChatSessionState {
  messages: AgentMessage[];
  input: string;
  running: boolean;
  streamingText: string;
  streamingThinking: string;
  pendingQuestion: PendingQuestion | null;
  pendingPermissions: PendingPermission[];
  liveToolResults: ToolResult[];
  activeToolCalls: ToolCall[];
  todoItems: TodoItem[];
  runActivity: RunActivityState;
  attachments: ImageAttachment[];
}

export interface ChatStoreState {
  sessions: Record<string, ChatSessionState>;
  loadMessages: (sessionId: string, messages: AgentMessage[]) => void;
  setInput: (sessionId: string, value: string) => void;
  pickImages: (sessionId: string) => Promise<void>;
  removeAttachment: (sessionId: string, index: number) => void;
  addAttachments: (sessionId: string, attachments: ImageAttachment[]) => void;
  sendMessage: (sessionId: string) => Promise<void>;
  abort: (sessionId: string) => Promise<void>;
  answerQuestion: (
    sessionId: string,
    requestId: string,
    answer: string | string[],
  ) => Promise<void>;
  approvePermission: (sessionId: string, requestId: string, allow: boolean) => Promise<void>;
  clear: (sessionId: string) => void;
  handleEvent: (sessionId: string, event: AgentSessionEvent) => void;
}
