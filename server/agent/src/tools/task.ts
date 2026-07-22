/**
 * Subagent Task Tool ('task').
 * Spawns an isolated child AgentLoop to execute a focused sub-task without cluttering the main conversation history.
 * Inspired by oh-my-pi/packages/coding-agent/src/task/ & oh-my-pi/packages/agent/src/agent-loop.ts.
 */
import { z } from "zod";
import { agentLoop, type StreamFn } from "../service/agent-loop.js";
import type { AgentTool, Model } from "../types/index.js";

export interface TaskToolContext {
  model: Model;
  streamFn: StreamFn;
  tools: AgentTool[];
  systemPrompt?: string;
}

let activeTaskContext: TaskToolContext | undefined;

export function setTaskToolContext(ctx?: TaskToolContext): void {
  activeTaskContext = ctx;
}

const inputSchema = z.object({
  prompt: z.string().describe("Clear, actionable task description for the subagent"),
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

export const taskTool: AgentTool<typeof inputSchema> = {
  name: "task",
  description: `Spawn an isolated subagent to execute a dedicated sub-task (e.g. searching files, inspecting tests, evaluating code).
The subagent runs in its own memory context and returns its final summary result back to you.`,
  tier: "read",
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    const { prompt, role, maxTurns = 10 } = args;

    if (!activeTaskContext) {
      return {
        content: [
          {
            type: "text",
            text: `Subagent [${role}] simulated run for: "${prompt}"\n(No active StreamFn/model attached to task tool context)`,
          },
        ],
      };
    }

    const { model, streamFn, tools, systemPrompt = "" } = activeTaskContext;
    const subagentSystemPrompt = `You are a specialized subagent (${role}). Execute the task thoroughly and summarize your findings cleanly.\n${systemPrompt}`;

    const stream = agentLoop(prompt, {
      model,
      systemPrompt: subagentSystemPrompt,
      tools: tools.filter((t) => t.name !== "task"), // Prevent infinite recursion
      streamFn,
      maxTurns,
      approvalMode: "accept-edits",
    });

    const messages = await stream.result();
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
          text: `Subagent [${role}] Completed Task:\n${summaryText.trim() || "Subagent finished with no text output."}`,
        },
      ],
    };
  },
};
