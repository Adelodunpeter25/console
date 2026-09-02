import type { AssistantMessage } from "./agent";
import type { PermissionRequest, ToolCall, ToolCallPreview, ToolResult } from "./tool";
import type { TodoItem } from "./todo";

export interface AskQuestionRequest {
  requestId: string;
  question: string;
  /** Optional multiple-choice options; omit for a free-text question. */
  options?: string[];
  isMultiSelect?: boolean;
  /** When true, the user may skip the question entirely. */
  skippable?: boolean;
  /** Shared id grouping questions from a single askMany call. */
  batchId?: string;
}

export interface SubagentStartEvent {
  type: "subagentStart";
  subagentId: string;
  parentToolCallId: string;
  name: string;
  role: string;
  prompt: string;
  maxTurns: number;
}

export interface SubagentActivityEvent {
  type: "subagentActivity";
  subagentId: string;
  turnIndex: number;
  toolCallId: string;
  toolName: string;
  args?: Record<string, unknown>;
  status: "running" | "completed" | "error";
  error?: string;
}

export interface SubagentEndEvent {
  type: "subagentEnd";
  subagentId: string;
  status: "completed" | "aborted" | "error";
  summary?: string;
  error?: string;
  totalTurns: number;
}

export type SubagentEvent =
  | SubagentStartEvent
  | SubagentActivityEvent
  | SubagentEndEvent;

export interface SubagentActivityItem {
  turnIndex: number;
  toolCallId: string;
  toolName: string;
  args?: Record<string, unknown>;
  status: "running" | "completed" | "error";
  error?: string;
}

export interface SubagentInfo {
  subagentId: string;
  parentToolCallId: string;
  name: string;
  role: string;
  prompt: string;
  maxTurns: number;
  currentTurn: number;
  status: "running" | "completed" | "aborted" | "error";
  summary?: string;
  error?: string;
  activities: SubagentActivityItem[];
  createdAt?: number;
  updatedAt?: number;
}

export type AgentSessionEvent =
  | { type: "sessionStart" }
  | { type: "turnStart"; prompt: string }
  | { type: "modelStreamStart"; turnId: string }
  | { type: "modelStreamPart"; part: { text?: string; thinking?: string; toolCall?: ToolCallPreview } }
  | { type: "modelStreamEnd"; turnId: string; turn: AssistantMessage }
  | { type: "toolExecutionStart"; calls: ToolCall[] }
  | { type: "permissionRequest"; request: PermissionRequest }
  | { type: "askQuestion"; request: AskQuestionRequest }
  | { type: "toolExecutionResult"; result: ToolResult }
  | { type: "toolExecutionEnd"; results: ToolResult[] }
  | { type: "todoUpdate"; items: TodoItem[]; action: "created" | "updated" }
  | { type: "compaction"; summary: string; originalMessageCount: number }
  | { type: "turnEnd"; turnId: string }
  | { type: "sessionEnd" }
  | { type: "error"; error: { message: string; data?: unknown } }
  | SubagentStartEvent
  | SubagentActivityEvent
  | SubagentEndEvent
  /** Synthetic frame (re-attach streams only): run completed. */
  | { type: "done"; summary?: string }
  /** Synthetic frame (re-attach streams only): run was aborted. */
  | { type: "aborted"; reason?: string }
  /** Synthetic frame (re-attach streams only): precedes a replay batch so clients clear streaming buffers first. */
  | { type: "streamReset" };

export interface SseEventFrame {
  event: AgentSessionEvent["type"];
  data: AgentSessionEvent;
}
