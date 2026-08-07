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

export type RunStatus = "working" | "completed" | "aborted" | "failed";

/** A single entry in the run activity timeline. */
export type ActivityEvent =
  | { type: "text"; id: string; text: string }
  | { type: "thinking"; id: string; text: string }
  | { type: "toolCall"; id: string; call: ToolCall; result?: ToolResult };

export interface RunActivityState {
  runId: string;
  startedAt: number | null;
  elapsedMs: number;
  events: ActivityEvent[];
  status: RunStatus;
}

export interface ChatSessionState {
  messages: AgentMessage[];
  input: string;
  running: boolean;
  streamingText: string;
  streamingThinking: string;
  pendingQuestion: PendingQuestion | null;
  pendingPermissions: PendingPermission[];
  activeToolCalls: ToolCall[];
  todoItems: TodoItem[];
  runs: RunActivityState[];
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
