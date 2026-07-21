import type { ToolCall, ToolResult } from "./tool.js";

export type MessageRole = "user" | "assistant" | "toolResult";

export interface TextPart {
  type: "text";
  text: string;
}

export interface ToolCallPart {
  type: "toolCall";
  call: ToolCall;
}

export interface UserMessage {
  role: "user";
  content: string; // User content is always a single string
}

export interface AssistantMessage {
  role: "assistant";
  id: string; // Unique ID for the assistant's turn
  content: (TextPart | ToolCallPart)[]; // Can contain text and tool calls
  stopReason: "stop" | "toolUse" | "error" | "maxTokens";
}

export interface ToolResultMessage {
  role: "toolResult";
  results: ToolResult[];
}

/**
 * A discriminated union representing all possible messages that can be part
 * of a conversation history. This structure is more robust and closer to
* what production systems like oh-my-pi use.
 */
export type AgentMessage =
  | UserMessage
  | AssistantMessage
  | ToolResultMessage;
