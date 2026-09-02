import { randomUUID } from "node:crypto";
import { z } from "zod";
import { agentLoop, type StreamFn } from "@/agent/src/service/agent-loop.js";
import type { AgentSessionEvent, AgentTool, ApprovalMode, Model, PermissionRequest } from "@/agent/src/types/index.js";

export interface SubagentToolContext {
  model: Model;
  streamFn: StreamFn;
  tools: AgentTool[];
  systemPrompt?: string;
  approvalMode?: ApprovalMode;
  onApproval?: (request: PermissionRequest) => Promise<boolean> | boolean;
  onEvent?: (event: AgentSessionEvent) => void;
}

const inputSchema = z.object({
  prompt: z.string().describe("Clear, actionable task description for the subagent"),
  name: z
    .string()
    .describe("Name or identifier for the subagent to help differentiate it from other instances"),
  role: z
    .string()
    .optional()
    .default("Subagent Researcher")
    .describe("Role or job title for the subagent (e.g. 'Codebase Inspector', 'Test Runner')"),
});

type Input = z.infer<typeof inputSchema>;

const description = "Delegate a focused sub-task to an isolated subagent.";

// Widened return type so the tool slots uniformly into AgentTool<ZodTypeAny>
// collections without zod generic-variance friction.
export function createSubagentTool(context?: SubagentToolContext): AgentTool {
  return {
    name: "subagent",
    description,
    tier: "read",
    inputSchema,
    execute: async (args: Input, signal?: AbortSignal, callId?: string): Promise<unknown> => {
      const { prompt, name, role = "Subagent Researcher" } = args;
      const displayName = name || role;
      const subagentId = `subagent-${randomUUID()}`;
      const parentToolCallId = callId || "";
      let turnIndex = 0;
      let totalTurns = 0;

      if (!context) {
        return {
          content: [
            {
              type: "text",
              text: `Subagent [${displayName}] simulated run for: "${prompt}"\n(No active StreamFn/model attached to task tool context)`,
            },
          ],
        };
      }

      const {
        model,
        streamFn,
        tools,
        systemPrompt = "",
        approvalMode = "always-ask",
        onApproval,
        onEvent,
      } = context;
      const subagentSystemPrompt = `You are a specialized subagent (${role}). Execute the task thoroughly and summarize your findings cleanly.\n${systemPrompt}`;

      onEvent?.({
        type: "subagentStart",
        subagentId,
        parentToolCallId,
        name: displayName,
        role,
        prompt,
        maxTurns: 10,
      });

      try {
        const stream = agentLoop(prompt, {
          model,
          systemPrompt: subagentSystemPrompt,
          tools: tools.filter((t) => t.name !== "subagent"),
          streamFn,
          approvalMode,
          onApproval,
          signal,
          onEvent: (event) => {
            if (event.type === "turnStart") {
              turnIndex++;
              totalTurns = turnIndex;
            } else if (event.type === "toolExecutionStart") {
              for (const call of event.calls) {
                onEvent?.({
                  type: "subagentActivity",
                  subagentId,
                  turnIndex,
                  toolCallId: call.id,
                  toolName: call.name,
                  args: call.arguments && typeof call.arguments === "object" ? (call.arguments as Record<string, unknown>) : undefined,
                  status: "running",
                });
              }
            } else if (event.type === "toolExecutionResult") {
              onEvent?.({
                type: "subagentActivity",
                subagentId,
                turnIndex,
                toolCallId: event.result.toolCallId,
                toolName: event.result.toolName || "",
                status: event.result.isError ? "error" : "completed",
                error: event.result.isError
                  ? typeof event.result.content === "string"
                    ? event.result.content
                    : JSON.stringify(event.result.content)
                  : undefined,
              });
            }
          },
        });

        const messages = await stream.result();

        if (signal?.aborted) {
          onEvent?.({
            type: "subagentEnd",
            subagentId,
            status: "aborted",
            summary: `Subagent [${displayName}] cancelled by user abort.`,
            totalTurns,
          });
          return {
            content: [
              {
                type: "text",
                text: `Subagent [${displayName}] cancelled by user abort.`,
              },
            ],
            isError: true,
          };
        }

        const lastAssistantMessage = [...messages].reverse().find((m) => m.role === "assistant");

        let summaryText = "";
        if (lastAssistantMessage) {
          for (const part of lastAssistantMessage.content) {
            if (part.type === "text") {
              summaryText += part.text + "\n";
            }
          }
        }

        const finalSummary = summaryText.trim() || "Subagent finished with no text output.";

        onEvent?.({
          type: "subagentEnd",
          subagentId,
          status: "completed",
          summary: finalSummary,
          totalTurns,
        });

        return {
          content: [
            {
              type: "text",
              text: `Subagent [${displayName}] Completed Task:\n${finalSummary}`,
            },
          ],
        };
      } catch (err) {
        const errorMsg = err instanceof Error ? err.message : String(err);
        onEvent?.({
          type: "subagentEnd",
          subagentId,
          status: "error",
          error: errorMsg,
          totalTurns,
        });
        throw err;
      }
    },
  };
}

/** Backward-compatible export name; the model-facing tool is `subagent`. */
export const subagentTool = createSubagentTool();
export const taskTool = subagentTool;
export const createTaskTool = createSubagentTool;
