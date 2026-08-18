import { randomUUID } from "node:crypto";
import { resolveApproval } from "../permissions/approval.js";
import { normalizeToolOutput } from "../utils/tool-output.js";
import { validateToolInput } from "./tool-input.js";
import type { ToolInputTelemetry } from "./tool-input.js";
import type { AgentTool, AgentSessionEvent, ApprovalMode, PermissionRequest, ToolCall, ToolResult } from "../types/index.js";
import type { AgentLoopConfig } from "./types.js";
import type { Model } from "@console/types";

/**
 * Execute a single tool call with Zod parsing, Permission resolution, & error handling.
 */
export async function executeTool(
  call: ToolCall,
  tools: AgentTool[],
  approvalMode: ApprovalMode,
  onApproval: AgentLoopConfig["onApproval"],
  emit: (event: AgentSessionEvent) => void,
  onToolCall?: AgentLoopConfig["onToolCall"],
  onToolResult?: AgentLoopConfig["onToolResult"],
  signal?: AbortSignal,
  model?: Model,
  onToolInputEvent?: AgentLoopConfig["onToolInputEvent"],
  toolInputTelemetry?: ToolInputTelemetry,
): Promise<ToolResult> {
  const tool = tools.find((t) => t.name === call.name);

  if (!tool) {
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: `Tool "${call.name}" is not registered. Available tools: ${tools.map((t) => t.name).join(", ")}`,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }

  // Validate raw arguments first. Repair only the exact failed paths when the
  // initial parse rejects recoverable model-shaped input.
  const validation = validateToolInput(tool.inputSchema, call.arguments);
  if (!validation.success) {
    const issues = validation.issues ?? [];
    const event = {
      name: "tool_input_invalid",
      label: `tool_input_invalid:${tool.name}`,
      toolName: tool.name,
      modelId: model?.id,
      provider: model?.provider,
      repairedPaths: validation.repairedPaths,
    } as const;
    toolInputTelemetry?.record(event);
    await onToolInputEvent?.(event);
    const errorText = issues
      .map((issue) => `  - ${issue.path.join(".")}: ${issue.message}`)
      .join("\n");
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content:
        `The arguments for tool "${call.name}" could not be used. ` +
        `Retry with the exact schema shape:\n${errorText}`,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }

  if (validation.repairedPaths.length > 0) {
    const event = {
      name: "tool_input_repaired",
      label: `tool_input_repaired:${tool.name}`,
      toolName: tool.name,
      modelId: model?.id,
      provider: model?.provider,
      repairedPaths: [...new Set(validation.repairedPaths)],
    } as const;
    toolInputTelemetry?.record(event);
    await onToolInputEvent?.(event);
  }

  const parsedData = validation.data as Parameters<typeof tool.execute>[0];

  // Permissions & Approval resolution
  const approval = resolveApproval(tool, parsedData, approvalMode);
  if (approval.policy === "deny") {
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: `Execution denied: ${approval.reason}`,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }

  if (approval.policy === "prompt") {
    const req: PermissionRequest = {
      requestId: randomUUID(),
      toolCallId: call.id,
      toolName: call.name,
      args: parsedData,
      tier: approval.tier,
      reason: approval.reason,
      ...(approvalMode === "plan-mode" ? { requiresUpgrade: true } : {}),
    };
    emit({ type: "permissionRequest", request: req });

    if (onApproval) {
      let allowed: boolean;
      try {
        allowed = await onApproval(req);
      } catch (err) {
        const result: ToolResult = {
          toolCallId: call.id,
          toolName: call.name,
          content:
            err instanceof Error
              ? err.message
              : "Tool execution cancelled before permission was granted.",
          isError: true,
        };
        await onToolResult?.(call, result);
        return result;
      }
      if (!allowed) {
        const result: ToolResult = {
          toolCallId: call.id,
          toolName: call.name,
          content: `Execution denied by user permission decision.`,
          isError: true,
        };
        await onToolResult?.(call, result);
        return result;
      }
    } else {
      // No approval handler registered — deny by default to prevent silent bypass.
      const result: ToolResult = {
        toolCallId: call.id,
        toolName: call.name,
        content: `Execution denied: no approval handler registered for tool '${call.name}' (${approval.tier} tier) in '${approvalMode}' mode.`,
        isError: true,
      };
      await onToolResult?.(call, result);
      return result;
    }
  }

  if (signal?.aborted) {
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: "Tool execution cancelled by user abort.",
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }

  try {
    await onToolCall?.(call);
    const output = await tool.execute(parsedData, signal);
    const normalized = normalizeToolOutput(output);
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      ...normalized,
    };
    await onToolResult?.(call, result);
    return result;
  } catch (err) {
    if (signal?.aborted) {
      const result: ToolResult = {
        toolCallId: call.id,
        toolName: call.name,
        content: "Tool execution cancelled by user abort.",
        isError: true,
      };
      await onToolResult?.(call, result);
      return result;
    }
    const message = err instanceof Error ? err.message : String(err);
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: message,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }
}
