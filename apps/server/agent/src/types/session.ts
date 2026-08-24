import type { AgentMessage, AssistantMessage } from "./message.js";
import type { Model } from "./model.js";
import type { AgentTool, PermissionRequest, ToolCall, ToolResult } from "./tool.js";
import type { TodoItem } from "@/agent/src/tools/todo.js";

/** Interactive question payload emitted when the model calls the 'ask' tool */
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

/**
 * Metadata header for a saved conversation session.
 */
export interface SessionHeader {
  id: string;
  title: string;
  cwd: string;
  modelId: string;
  provider: string;
  createdAt: number; // epoch ms
  updatedAt: number; // epoch ms
  messageCount?: number;
  deletedAt?: number;
}

/**
 * Represents the full context of a conversation at a specific point in time.
 */
export interface SessionContext {
  model: Model;
  messages: AgentMessage[];
  tools: AgentTool[];
}

/**
 * A discriminated union representing the rich lifecycle events of the agent.
 */
export type AgentSessionEvent =
  | { type: "sessionStart" }
  | { type: "turnStart"; prompt: string }
  | { type: "modelStreamStart"; turnId: string }
  | { type: "modelStreamPart"; part: { text?: string; thinking?: string; toolCall?: ToolCall } }
  | { type: "modelStreamEnd"; turn: AssistantMessage }
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
