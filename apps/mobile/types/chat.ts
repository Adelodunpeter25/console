import type {
  AgentMessage,
  AgentSessionEvent,
  PermissionRequest,
  AskQuestionRequest,
  ToolCall,
  ToolResult,
} from "@console/types";

/** A pending permission decision surfaced to the user for allow/deny. */
export interface PendingPermission {
  request: PermissionRequest;
}

/** A pending ask-tool question surfaced to the user with options. */
export interface PendingQuestion {
  request: AskQuestionRequest;
}

/** The live, client-side state of a chat session while streaming runs. */
export interface ChatSnapshot {
  messages: AgentMessage[];
  streamingText: string;
  streamingThinking: string;
  activeToolCalls: ToolCall[];
  liveToolResults: ToolResult[];
  pendingPermission: PendingPermission | null;
  pendingQuestion: PendingQuestion | null;
  running: boolean;
}

export const emptyChatSnapshot: ChatSnapshot = {
  messages: [],
  streamingText: "",
  streamingThinking: "",
  activeToolCalls: [],
  liveToolResults: [],
  pendingPermission: null,
  pendingQuestion: null,
  running: false,
};

/** Reducer contract: given the current snapshot and an agent event, produce the next snapshot. */
export type ChatEventReducer = (snapshot: ChatSnapshot, event: AgentSessionEvent) => ChatSnapshot;
