import type { z } from "zod";

export type ToolTier = "read" | "write" | "exec";

export type ApprovalMode = "always-ask" | "accept-edits" | "plan-mode" | "full-access";

export type ApprovalPolicy = "allow" | "deny" | "prompt";

export interface PermissionRequest {
  requestId: string;
  toolCallId: string;
  toolName: string;
  args: unknown;
  tier: ToolTier;
  reason?: string;
}

export interface ToolCall {
  id: string;
  name: string;
  arguments: unknown;
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
