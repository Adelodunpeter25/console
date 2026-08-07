/**
 * Subagent Task Tool ('subagent').
 * Spawns an isolated child AgentLoop to execute a focused sub-task without cluttering the main conversation history.
 */
import { z } from "zod";
import { agentLoop, type StreamFn } from "../service/agent-loop.js";
import type { AgentTool, Model } from "../types/index.js";

export interface SubagentToolContext {
  model: Model;
  streamFn: StreamFn;
  tools: AgentTool[];
  systemPrompt?: string;
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
  maxTurns: z
    .number()
    .int()
    .min(1)
    .max(20)
    .optional()
    .default(10)
    .describe("Maximum turns for the subagent run"),
});

type Input = z.infer<typeof inputSchema>;

const description = `Spawn an isolated subagent to execute a dedicated sub-task (e.g. searching files, inspecting tests, evaluating code).
The subagent runs in its own memory context and returns its final summary result back to you.`;

export function createSubagentTool(context?: SubagentToolContext): AgentTool<typeof inputSchema> {
  return {
    name: "subagent",
    description,
    tier: "read",
    inputSchema,
    execute: async (args: Input, signal?: AbortSignal): Promise<unknown> => {
      const { prompt, name, role, maxTurns = 10 } = args;
      const displayName = name || role;

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

      const { model, streamFn, tools, systemPrompt = "" } = context;
      const subagentSystemPrompt = `You are a specialized subagent (${role}). Execute the task thoroughly and summarize your findings cleanly.\n${systemPrompt}`;

      const stream = agentLoop(prompt, {
        model,
        systemPrompt: subagentSystemPrompt,
        tools: tools.filter((t) => t.name !== "subagent"),
        streamFn,
        maxTurns,
        approvalMode: "accept-edits",
        signal,
      });

      const messages = await stream.result();

      if (signal?.aborted) {
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

      return {
        content: [
          {
            type: "text",
            text: `Subagent [${displayName}] Completed Task:\n${summaryText.trim() || "Subagent finished with no text output."}`,
          },
        ],
      };
    },
  };
}

/** Backward-compatible export name; the model-facing tool is `subagent`. */
export const subagentTool = createSubagentTool();
export const taskTool = subagentTool;
export const createTaskTool = createSubagentTool;
