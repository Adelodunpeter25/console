import type {
  AgentMessage,
  AgentSessionEvent,
  AgentTool,
  ApprovalMode,
  Model,
  PermissionRequest,
  ToolCall,
  ToolResult,
} from "@/agent/src/types/index.js";
import type { CompactionOptions } from "@/agent/src/compaction/index.js";

export type LLMDelta =
  | { type: "text"; text: string; thoughtSignature?: string }
  | { type: "thinking"; text: string }
  | {
      type: "toolCall";
      id: string;
      name: string;
      argumentsJson: string;
      thoughtSignature?: string;
    };

export type StreamFn = (params: {
  model: Model;
  systemPrompt: string;
  messages: AgentMessage[];
  tools: AgentTool[];
  signal?: AbortSignal;
}) => AsyncIterable<LLMDelta>;

export interface AgentLoopConfig {
  /** The model to use. */
  model: Model;
  /** System prompt sent on every request. */
  systemPrompt: string;
  /** Tools available to the agent. */
  tools: AgentTool[];
  /** Provider-specific streaming function — inject your Gemini/Antigravity client here. */
  streamFn: StreamFn;
  /** Security approval mode ("always-ask" | "accept-edits" | "plan-mode" | "full-access"). Default: "always-ask" */
  approvalMode?: ApprovalMode;
  /** Hook for user approval when a tool call requires permission. */
  onApproval?: (request: PermissionRequest) => Promise<boolean> | boolean;
  /**
   * Called for every AgentSessionEvent as it is emitted.
   * Useful for logging, UI updates, or WebSocket forwarding.
   */
  onEvent?: (event: AgentSessionEvent) => void;
  /** AbortSignal to cancel the run. */
  signal?: AbortSignal;
  /** Compaction options for automated history summarization. */
  compaction?: CompactionOptions;
  /** Hook called before a tool is executed. Useful for approval flows. */
  onToolCall?: (call: ToolCall) => Promise<void> | void;
  /** Hook called after a tool finishes executing. */
  onToolResult?: (call: ToolCall, result: ToolResult) => Promise<void> | void;
  /** Maximum agentic turns (LLM calls) before the loop stops. */
  maxTurns?: number;
}

export interface StreamParams {
  model: Model;
  systemPrompt: string;
  messages: AgentMessage[];
  tools: AgentTool[];
  signal?: AbortSignal;
}
