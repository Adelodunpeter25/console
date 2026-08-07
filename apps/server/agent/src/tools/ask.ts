/**
 * Interactive Question Tool ('ask').
 * Allows the agent to ask the user structured multiple-choice questions to clarify requirements.
 * Inspired by oh-my-pi/packages/coding-agent/src/tools/ask.ts.
 */
import { randomUUID } from "node:crypto";
import { z } from "zod";
import type { AgentTool, AskQuestionRequest } from "../types/index.js";

export type AskQuestionHandler = (request: AskQuestionRequest) => Promise<string | string[]>;

const inputSchema = z.object({
  question: z.string().describe("The question to ask the user"),
  options: z
    .array(z.string())
    .optional()
    .describe("Optional list of selectable multiple-choice options for the user. Omit for a free-text question."),
  isMultiSelect: z
    .boolean()
    .optional()
    .default(false)
    .describe("Set to true if user can select multiple options"),
  skippable: z
    .boolean()
    .optional()
    .default(false)
    .describe("Set to true if the question is optional and the user may skip it"),
});

type Input = z.infer<typeof inputSchema>;

const description = `Ask the user a question to clarify ambiguous requirements, architecture options, or preferences.
Provide a clear question and, optionally, a list of distinct option choices. The user can always type a custom answer, so plain free-text questions are fine too.`;

export function createAskTool(handler?: AskQuestionHandler): AgentTool<typeof inputSchema> {
  return {
    name: "ask",
    description,
    tier: "read",
    inputSchema,
    execute: async (args: Input): Promise<unknown> => {
      const { question, options, isMultiSelect = false, skippable = false } = args;

      const request: AskQuestionRequest = {
        requestId: randomUUID(),
        question,
        options,
        isMultiSelect,
        skippable,
      };

      if (!handler) {
        // Default fallback when running headless or unattached
        const defaultAnswer = options?.[0] ?? "skipped";
        return {
          content: [
            {
              type: "text",
              text: `[Asked User]: "${question}"\nSelected default option: "${defaultAnswer}"`,
            },
          ],
        };
      }

      const answer = await handler(request);
      const answerText = Array.isArray(answer) ? answer.join(", ") : answer;

      return {
        content: [
          {
            type: "text",
            text:
              answerText === ""
                ? `[User Answer]: User skipped this question.`
                : `[User Answer]: "${answerText}"`,
          },
        ],
      };
    },
  };
}

export const askTool = createAskTool();
