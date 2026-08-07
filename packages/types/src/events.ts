import type { AssistantMessage } from "./agent";
import type { PermissionRequest, ToolCall, ToolResult } from "./tool";
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

export type AgentSessionEvent =
  | { type: "sessionStart" }
  | { type: "turnStart"; prompt: string }
  | { type: "modelStreamStart"; turnId: string }
  | { type: "modelStreamPart"; part: { text?: string; thinking?: string; toolCall?: ToolCall } }
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
  | { type: "error"; error: { message: string; data?: unknown } };

export interface SseEventFrame {
  event: AgentSessionEvent["type"];
  data: AgentSessionEvent;
}
