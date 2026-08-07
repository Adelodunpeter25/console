/**
 * Interactive Question Tools ('ask' / 'askMany').
 * Allow the agent to ask the user structured multiple-choice or free-text
 * questions to clarify requirements.
 * Inspired by oh-my-pi/packages/coding-agent/src/tools/ask.ts.
 */
import { randomUUID } from "node:crypto";
import { z } from "zod";
import type { AgentTool, AskQuestionRequest } from "../types/index.js";

export type AskQuestionHandler = (request: AskQuestionRequest) => Promise<string | string[]>;

const questionSchema = z.object({
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
    .default(true)
    .describe("Set to false if this question is REQUIRED and cannot be skipped by the user. Defaults to true (optional)."),
});

const inputSchema = questionSchema;

type Input = z.infer<typeof inputSchema>;

const description = `Ask the user a question to clarify ambiguous requirements, architecture options, or preferences.
Provide a clear question and, optionally, a list of distinct option choices. Set skippable to false if the question is strictly required.`;

/** Build the tool-result text for a single answer, marking skips explicitly. */
function formatAnswer(answer: string | string[]): string {
  const text = Array.isArray(answer) ? answer.join(", ") : answer;
  return text === ""
    ? `[User Answer]: User skipped this question.`
    : `[User Answer]: "${text}"`;
}

/** Emit one askQuestion request and await its answer via the handler. */
function askOne(
  handler: AskQuestionHandler,
  input: Input,
  batchId?: string,
): Promise<string | string[]> {
  const isSkippable = input.skippable !== false;
  const request: AskQuestionRequest = {
    requestId: randomUUID(),
    question: input.question,
    options: input.options,
    isMultiSelect: input.isMultiSelect,
    skippable: isSkippable,
    ...(batchId ? { batchId } : {}),
  };
  return handler(request);
}

export function createAskTool(handler?: AskQuestionHandler): AgentTool<typeof inputSchema> {
  return {
    name: "ask",
    description,
    tier: "read",
    inputSchema,
    execute: async (args: Input): Promise<unknown> => {
      const { question, options } = args;

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

      const answer = await askOne(handler, args);
      return { content: [{ type: "text", text: formatAnswer(answer) }] };
    },
  };
}

export const askTool = createAskTool();

/* ------------------------------------------------------------------ */
/* askMany — batch question tool                                       */
/* ------------------------------------------------------------------ */

const askManySchema = z.object({
  questions: z
    .array(questionSchema)
    .min(1)
    .describe("The list of questions to ask the user, answered in order"),
});

const askManyDescription = `Ask the user multiple questions at once, presented one at a time.
Each question may have optional multiple-choice options; the user can always type a custom answer or skip (if skippable). Use this when you need several independent answers before proceeding.`;

export function createAskManyTool(handler?: AskQuestionHandler): AgentTool<typeof askManySchema> {
  return {
    name: "askMany",
    description: askManyDescription,
    tier: "read",
    inputSchema: askManySchema,
    execute: async (args: z.infer<typeof askManySchema>): Promise<unknown> => {
      if (!handler) {
        // Headless fallback: pick the first option (or "skipped") per question.
        return {
          content: [
            {
              type: "text",
              text: args.questions
                .map((q) => {
                  const defaultAnswer = q.options?.[0] ?? "skipped";
                  return `[Asked User]: "${q.question}"\nSelected default option: "${defaultAnswer}"`;
                })
                .join("\n\n"),
            },
          ],
        };
      }

      const batchId = randomUUID();
      const answers: Array<string | string[]> = [];
      for (const q of args.questions) {
        answers.push(await askOne(handler, q, batchId));
      }

      return {
        content: [
          {
            type: "text",
            text: answers
              .map((a, i) => `Q${i + 1}: ${formatAnswer(a)}`)
              .join("\n"),
          },
        ],
      };
    },
  };
}

export const askManyTool = createAskManyTool();
