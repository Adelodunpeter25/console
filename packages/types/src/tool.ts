import { z } from "zod";

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

/** Partial tool-call payload emitted while model arguments are still streaming. */
export interface ToolCallPreview {
  id: string;
  name: string;
  arguments?: unknown;
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
  execute: (args: z.infer<T>, signal?: AbortSignal) => Promise<unknown>;
}

export class ToolError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ToolError";
  }
}

/**
 * Wrap a tool for a project session. The model-facing schema omits `cwd` so
 * the model only supplies the target path; execution still receives the
 * selected project directory internally.
 *
 * Tools like `glob` and `bash` fall back to `process.cwd()` when no `cwd` arg is
 * given, which is the server's launch directory — not the session's project.
 * Binding the session cwd here makes every run operate on the project the user
 * selected in the UI instead of the server's working directory.
 */
export function bindToolCwd<T extends z.ZodTypeAny>(
  tool: AgentTool<T>,
  cwd: string,
): AgentTool<T> {
  const hasCwdArg = tool.inputSchema instanceof z.ZodObject && "cwd" in tool.inputSchema.shape;
  if (!hasCwdArg) {
    return tool;
  }

  const originalExecute = tool.execute;
  const modelInputSchema = (tool.inputSchema as unknown as z.AnyZodObject).omit({ cwd: true });
  return {
    ...tool,
    inputSchema: modelInputSchema as unknown as T,
    execute: async (args, signal) => originalExecute({ ...args, cwd } as z.infer<T>, signal),
  };
}
