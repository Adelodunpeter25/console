import type { AgentMessage, AssistantMessage } from "./message.js";
import type { Model } from "./model.js";
import type { AgentTool, ToolCall, ToolResult } from "./tool.js";

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
 * This allows for detailed observation of the agent's state.
 */
export type AgentSessionEvent =
  | { type: "sessionStart" }
  | { type: "turnStart"; prompt: string }
  | { type: "modelStreamStart"; turnId: string }
  | { type: "modelStreamPart"; part: { text?: string; toolCall?: ToolCall } }
  | { type: "modelStreamEnd"; turn: AssistantMessage }
  | { type: "toolExecutionStart"; calls: ToolCall[] }
  | { type: "toolExecutionResult"; result: ToolResult }
  | { type: "toolExecutionEnd"; results: ToolResult[] }
  | { type: "turnEnd"; turnId: string }
  | { type: "sessionEnd" }
  | { type: "error"; error: { message: string; data?: unknown } };

