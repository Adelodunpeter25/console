import type { z } from "zod";

export type ToolTier = "read" | "write" | "exec";

export type ApprovalMode = "always-ask" | "accept-edits" | "plan-mode" | "full-access";

/** Approval mode metadata served by the backend for dynamic UI rendering. */
export interface ApprovalModeOption {
  value: ApprovalMode;
  label: string;
  description: string;
}

export type ApprovalPolicy = "allow" | "deny" | "prompt";

export interface PermissionRequest {
  requestId: string;
  toolCallId: string;
  toolName: string;
  args: unknown;
  tier: ToolTier;
  reason?: string;
  /** True when the current mode must be escalated for this one action. */
  requiresUpgrade?: boolean;
}

export interface ToolCall {
  id: string;
  name: string;
  arguments: unknown;
  /** Opaque Gemini thought signature returned with this function call. */
  thoughtSignature?: string;
}

export interface ToolResult {
  toolCallId: string;
  toolName?: string;
  content: unknown;
  isError?: boolean;
}

export interface AgentTool<T extends z.ZodTypeAny = z.ZodTypeAny> {
  name: string;
  description: string;
  tier?: ToolTier;
  inputSchema: T;
  execute: (args: z.infer<T>) => Promise<unknown>;
}

export class ToolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ToolError";
  }
}
