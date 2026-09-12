import type { ToolCall } from "./tool";
import type { TurnUsage } from "./cache";

export interface TextPart {
  type: "text";
  text: string;
  /** Opaque Gemini thought signature returned with this model part. */
  thoughtSignature?: string;
}

export interface ThinkingPart {
  type: "thinking";
  text: string;
}

export interface ToolCallPart {
  type: "toolCall";
  call: ToolCall;
}

export interface ImagePart {
  type: "image";
  /** Base64-encoded image data (no data: prefix). */
  data: string;
  /** MIME type, e.g. "image/png". */
  mimeType: string;
}

export type AssistantMessageContent = TextPart | ThinkingPart | ToolCallPart;

export interface UserMessage {
  role: "user";
  /** Unique per-message id (assigned by clients for keyed array storage). */
  id?: string;
  content: string;
  /** Inline image attachments sent with the prompt (base64-encoded). */
  attachments?: ImagePart[];
}

export interface AssistantMessage {
  role: "assistant";
  id?: string;
  content: AssistantMessageContent[];
  stopReason?: "stop" | "toolUse" | "maxTokens" | "aborted";
  /** Normalized per-turn usage with cache breakdown (when the provider reports it). */
  usage?: TurnUsage;
}

export interface ToolResultMessage {
  role: "toolResult";
  /** Unique per-message id (assigned by clients for keyed array storage). */
  id?: string;
  results: import("./tool").ToolResult[];
}

export type AgentMessage = (UserMessage | AssistantMessage | ToolResultMessage) & {
  createdAt?: number;
};
