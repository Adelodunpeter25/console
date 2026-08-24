/**
 * Permissions & Tool Approval Resolution Engine.
 * Mirrors oh-my-pi/packages/coding-agent/src/tools/approval.ts.
 */
import type { AgentTool, ApprovalMode, ApprovalPolicy, ToolTier } from "@/agent/src/types/index.js";

const TIER_RANK: Record<ToolTier, number> = {
  read: 0,
  write: 1,
  exec: 2,
};

const MODE_MAX_ALLOWED_TIER: Record<ApprovalMode, ToolTier> = {
  "always-ask": "read",
  "accept-edits": "write",
  "plan-mode": "read",
  "full-access": "exec",
};

/**
 * Infer or resolve the capability tier of a tool.
 */
export function resolveToolTier(tool: Pick<AgentTool, "name" | "tier">, _args?: unknown): ToolTier {
  if (tool.tier) return tool.tier;

  const name = tool.name.toLowerCase();
  if (name.includes("write") || name.includes("edit")) return "write";
  if (name === "bash" || name.includes("exec")) return "exec";
  return "read";
}

export interface ResolvedApproval {
  policy: ApprovalPolicy;
  tier: ToolTier;
  reason?: string;
}

/**
 * Resolve approval policy for a tool execution under the given ApprovalMode.
 */
export function resolveApproval(
  tool: Pick<AgentTool, "name" | "tier">,
  args: unknown,
  mode: ApprovalMode = "always-ask",
): ResolvedApproval {
  const tier = resolveToolTier(tool, args);

  if (mode === "full-access") {
    return { policy: "allow", tier };
  }

  if (mode === "plan-mode") {
    if (tier === "read") {
      return { policy: "allow", tier };
    }
    return {
      policy: "prompt",
      tier,
      reason: `Tool '${tool.name}' requires upgraded permission because Plan Mode is read-only.`,
    };
  }

  const maxTier = MODE_MAX_ALLOWED_TIER[mode];
  if (TIER_RANK[tier] <= TIER_RANK[maxTier]) {
    return { policy: "allow", tier };
  }

  return {
    policy: "prompt",
    tier,
    reason: `Tool '${tool.name}' (${tier} tier) requires user approval in '${mode}' mode.`,
  };
}
