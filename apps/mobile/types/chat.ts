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

/** The live, client-side state of a chat session while streaming runs. */
export interface ChatSessionState {
  messages: AgentMessage[];
  input: string;
  running: boolean;
  streamingText: string;
  streamingThinking: string;
  /** Queue of pending questions (a batch from askMany arrives all at once). */
  pendingQuestions: PendingQuestion[];
  pendingPermissions: PendingPermission[];
  activeToolCalls: ToolCall[];
  todoItems: TodoItem[];
  runs: RunActivityState[];
  attachments: ImageAttachment[];
  /** Last time draft (input/attachments) changed — for DRAFT sorting. */
  draftUpdatedAt?: number;
}

export interface ChatStoreState {
  sessions: Record<string, ChatSessionState>;
  loadMessages: (sessionId: string, messages: AgentMessage[]) => void;
  setInput: (sessionId: string, value: string) => void;
  addAttachments: (sessionId: string, attachments: ImageAttachment[]) => void;
  removeAttachment: (sessionId: string, index: number) => void;
  clearAttachments: (sessionId: string) => void;
  sendMessage: (sessionId: string, prompt?: string) => Promise<void>;
  abort: (sessionId: string) => Promise<void>;
  answerQuestion: (
    sessionId: string,
    requestId: string,
    answer: string | string[],
  ) => Promise<void>;
  approvePermission: (sessionId: string, requestId: string, allow: boolean) => Promise<void>;
  clear: (sessionId: string) => void;
  reset: (sessionId: string) => void;
  handleEvent: (sessionId: string, event: AgentSessionEvent) => void;
}

/** A snapshot of the currently selected session's chat state for the UI. */
export type ChatSnapshot = {
  messages: AgentMessage[];
  streamingText: string;
  streamingThinking: string;
  activeToolCalls: ToolCall[];
  liveToolResults: ToolResult[];
  pendingPermission: PendingPermission | null;
  pendingQuestion: PendingQuestion | null;
  pendingPermissions: PendingPermission[];
  pendingQuestions: PendingQuestion[];
  running: boolean;
  runs: RunActivityState[];
};

/** Reducer contract: given the current snapshot and an agent event, produce the next snapshot. */
export type ChatEventReducer = (snapshot: ChatSnapshot, event: AgentSessionEvent) => ChatSnapshot;