import type { AssistantMessage } from "./agent";
import type { PermissionRequest, ToolCall, ToolResult } from "./tool";

export interface AskQuestionRequest {
  requestId: string;
  question: string;
  options: string[];
  isMultiSelect?: boolean;
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
  | { type: "compaction"; summary: string; originalMessageCount: number }
  | { type: "turnEnd"; turnId: string }
  | { type: "sessionEnd" }
  | { type: "error"; error: { message: string; data?: unknown } };

export interface SseEventFrame {
  event: AgentSessionEvent["type"];
  data: AgentSessionEvent;
}
