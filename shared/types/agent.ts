import type { ToolCall } from "./tool.js";

export interface TextPart {
  type: "text";
  text: string;
}

export interface ThinkingPart {
  type: "thinking";
  text: string;
}

export interface ToolCallPart {
  type: "toolCall";
  call: ToolCall;
}

export type AssistantMessageContent = TextPart | ThinkingPart | ToolCallPart;

export interface UserMessage {
  role: "user";
  content: string;
}

export interface AssistantMessage {
  role: "assistant";
  id?: string;
  content: AssistantMessageContent[];
  stopReason?: "stop" | "toolUse" | "maxTokens" | "aborted";
}

export interface ToolResultMessage {
  role: "toolResult";
  results: import("./tool.js").ToolResult[];
}

export type AgentMessage = UserMessage | AssistantMessage | ToolResultMessage;
