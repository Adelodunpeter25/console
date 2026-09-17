import type {
  AgentMessage,
  AgentSessionEvent,
  AgentTool,
  ApprovalMode,
  CacheIdentity,
  CacheRetention,
  Model,
  ThinkingLevel,
  PermissionRequest,
  ToolCall,
  ToolResult,
  TurnUsage,
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
    }
  /**
   * Final cumulative usage for the turn. Providers should yield exactly one of
   * these per request — emitted as the last delta so callers can attach it to
   * the resulting AssistantMessage without parsing intermediate chunks.
   */
  | { type: "usage"; usage: TurnUsage };

export type StreamFn = (params: {
  model: Model;
  systemPrompt: string;
  messages: AgentMessage[];
  tools: AgentTool[];
  signal?: AbortSignal;
  thinkingLevel?: ThinkingLevel;
  /** Provider-neutral cache retention hint. Providers decide whether/how to honor it. */
  cacheRetention?: CacheRetention;
  /** Stable cache identity for this logical conversation. Omit when the provider cannot honor a stable key. */
  cacheIdentity?: CacheIdentity;
}) => AsyncIterable<LLMDelta>;

export type CompactionSummaryFn = (messages: AgentMessage[], signal?: AbortSignal) => Promise<string>;

export interface AgentLoopConfig {
  /** The model to use. */
  model: Model;
  /** System prompt sent on every request. */
  systemPrompt: string;
  /** Tools available to the agent. */
  tools: AgentTool[];
  /** Provider-specific streaming function — inject your Antigravity/Codex/Cline client here. */
  streamFn: StreamFn;
  /** Optional runtime thinking override for providers that support it. */
  thinkingLevel?: ThinkingLevel;
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
  /** Optional role-aware summary generator; failures should fall back structurally. */
  summarizeCompaction?: CompactionSummaryFn;
  /** Hook called before a tool is executed. Useful for approval flows. */
  onToolCall?: (call: ToolCall) => Promise<void> | void;
  /** Hook called after a tool finishes executing. */
  onToolResult?: (call: ToolCall, result: ToolResult) => Promise<void> | void;
  /**
   * Provider-neutral cache retention hint passed through to `streamFn`.
   * Defaults to "short" when not set; providers decide whether to honor it.
   */
  cacheRetention?: CacheRetention;
  /**
   * Stable cache identity passed through to `streamFn`. Should be stable for
   * the lifetime of one logical conversation (rotated when provider/model
   * changes).
   */
  cacheIdentity?: CacheIdentity;
}

export interface StreamParams {
  model: Model;
  systemPrompt: string;
  messages: AgentMessage[];
  tools: AgentTool[];
  signal?: AbortSignal;
  thinkingLevel?: ThinkingLevel;
  cacheRetention?: CacheRetention;
  cacheIdentity?: CacheIdentity;
}
