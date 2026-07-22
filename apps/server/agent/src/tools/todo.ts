/**
 * TODO List Management Tool.
 * Enables the agent to create, track, and update task lists across multi-turn runs.
 * Inspired by oh-my-pi/packages/coding-agent/src/tools/todo.ts.
 */
import { z } from "zod";
import type { AgentTool } from "../types/index.js";

export interface TodoItem {
  id: number;
  content: string;
  status: "pending" | "in_progress" | "completed";
}

let sessionTodoList: TodoItem[] = [];

export function getSessionTodoList(): readonly TodoItem[] {
  return sessionTodoList;
}

export function clearSessionTodoList(): void {
  sessionTodoList = [];
}

const inputSchema = z.object({
  op: z
    .enum(["init", "start", "done", "append", "view"])
    .describe("Operation to apply: 'init', 'start', 'done', 'append', or 'view'"),
  tasks: z
    .array(z.string())
    .optional()
    .describe("List of task item strings (used with 'init' or 'append')"),
  index: z
    .number()
    .int()
    .positive()
    .optional()
    .describe("Task ID/index (1-indexed) to update status for ('start' or 'done')"),
});

type Input = z.infer<typeof inputSchema>;

export const todoTool: AgentTool<typeof inputSchema> = {
  name: "todo",
  description: `Manage the session task list (TODOs) for multi-step feature development or refactoring.
Use 'init' with tasks: ["task 1", "task 2"] to initialize the task breakdown.
Use 'start' with index: 1 to mark a task as in_progress.
Use 'done' with index: 1 to mark a task as completed.
Use 'append' with tasks: ["new task"] to add items to the list.
Use 'view' to render the current task list status.`,
  tier: "read",
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    const { op, tasks, index } = args;

    if (op === "init") {
      if (!tasks || tasks.length === 0) {
        return {
          content: [
            { type: "text", text: "Error: 'init' operation requires a non-empty 'tasks' array." },
          ],
          isError: true,
        };
      }
      sessionTodoList = tasks.map((content, idx) => ({
        id: idx + 1,
        content,
        status: "pending",
      }));

      return renderTodoListResult("Initialized task list");
    }

    if (op === "append") {
      if (!tasks || tasks.length === 0) {
        return {
          content: [
            { type: "text", text: "Error: 'append' operation requires a non-empty 'tasks' array." },
          ],
          isError: true,
        };
      }
      const startId = sessionTodoList.length + 1;
      tasks.forEach((content, idx) => {
        sessionTodoList.push({
          id: startId + idx,
          content,
          status: "pending",
        });
      });

      return renderTodoListResult("Appended tasks");
    }

    if (op === "start") {
      if (index === undefined) {
        return {
          content: [{ type: "text", text: "Error: 'start' operation requires task 'index'." }],
          isError: true,
        };
      }
      const item = sessionTodoList.find((t) => t.id === index);
      if (!item) {
        return {
          content: [{ type: "text", text: `Error: Task index ${index} not found.` }],
          isError: true,
        };
      }
      item.status = "in_progress";
      return renderTodoListResult(`Marked task #${index} as in_progress`);
    }

    if (op === "done") {
      if (index === undefined) {
        return {
          content: [{ type: "text", text: "Error: 'done' operation requires task 'index'." }],
          isError: true,
        };
      }
      const item = sessionTodoList.find((t) => t.id === index);
      if (!item) {
        return {
          content: [{ type: "text", text: `Error: Task index ${index} not found.` }],
          isError: true,
        };
      }
      item.status = "completed";
      return renderTodoListResult(`Marked task #${index} as completed`);
    }

    // Default 'view'
    return renderTodoListResult("Current task list status");
  },
};

function renderTodoListResult(title: string) {
  if (sessionTodoList.length === 0) {
    return {
      content: [{ type: "text", text: `${title}:\n(No active tasks in TODO list)` }],
    };
  }

  const lines = sessionTodoList.map((item) => {
    const icon =
      item.status === "completed" ? "[x]" : item.status === "in_progress" ? "[>]" : "[ ]";
    return `${icon} #${item.id}: ${item.content} (${item.status})`;
  });

  return {
    content: [{ type: "text", text: `${title}:\n` + lines.join("\n") }],
  };
}
