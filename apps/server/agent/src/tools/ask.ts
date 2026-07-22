/**
 * Interactive Question Tool ('ask').
 * Allows the agent to ask the user structured multiple-choice questions to clarify requirements.
 * Inspired by oh-my-pi/packages/coding-agent/src/tools/ask.ts.
 */
import { randomUUID } from "node:crypto";
import { z } from "zod";
import type { AgentTool, AskQuestionRequest } from "../types/index.js";

export type AskQuestionHandler = (request: AskQuestionRequest) => Promise<string | string[]>;

let activeAskHandler: AskQuestionHandler | undefined;

export function setAskQuestionHandler(handler?: AskQuestionHandler): void {
  activeAskHandler = handler;
}

const inputSchema = z.object({
  question: z.string().describe("The question to ask the user"),
  options: z
    .array(z.string())
    .min(2)
    .describe("List of selectable multiple-choice options for the user"),
  isMultiSelect: z
    .boolean()
    .optional()
    .default(false)
    .describe("Set to true if user can select multiple options"),
});

type Input = z.infer<typeof inputSchema>;

export const askTool: AgentTool<typeof inputSchema> = {
  name: "ask",
  description: `Ask the user a structured multiple-choice question to clarify ambiguous requirements, architecture options, or preferences.
Provide a clear question and at least 2 distinct option choices.`,
  tier: "read",
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    const { question, options, isMultiSelect = false } = args;

    const request: AskQuestionRequest = {
      requestId: randomUUID(),
      question,
      options,
      isMultiSelect,
    };

    if (!activeAskHandler) {
      // Default fallback when running headless or unattached
      const defaultAnswer = options[0]!;
      return {
        content: [
          {
            type: "text",
            text: `[Asked User]: "${question}"\nSelected default option: "${defaultAnswer}"`,
          },
        ],
      };
    }

    const answer = await activeAskHandler(request);
    const answerText = Array.isArray(answer) ? answer.join(", ") : answer;

    return {
      content: [
        {
          type: "text",
          text: `[User Answer]: "${answerText}"`,
        },
      ],
    };
  },
};
