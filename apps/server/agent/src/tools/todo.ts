/**
 * Session-scoped TODO management tool.
 * The tool owns its list so concurrent agent sessions cannot overwrite each
 * other's tasks. The default singleton remains exported for offline tests.
 */
import { z } from "zod";
import type { AgentTool } from "../types/index.js";

export interface TodoItem {
  id: number;
  content: string;
  status: "pending" | "in_progress" | "completed";
}

export type TodoUpdateAction = "created" | "updated";
export type TodoUpdateHandler = (
  items: TodoItem[],
  action: TodoUpdateAction,
) => void | Promise<void>;

export interface TodoToolController {
  tool: AgentTool;
  getItems: () => readonly TodoItem[];
  clear: () => void;
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

export function createTodoTool(
  initialItems: readonly TodoItem[] = [],
  onUpdate?: TodoUpdateHandler,
): TodoToolController {
  let items = initialItems.map((item) => ({ ...item }));

  const publish = async (action: TodoUpdateAction) => {
    await onUpdate?.(
      items.map((item) => ({ ...item })),
      action,
    );
  };

  const render = (title: string) => {
    if (items.length === 0) {
      return { content: [{ type: "text", text: `${title}:\n(No active tasks in TODO list)` }] };
    }

    const lines = items.map((item) => {
      const icon =
        item.status === "completed" ? "[x]" : item.status === "in_progress" ? "[>]" : "[ ]";
      return `${icon} #${item.id}: ${item.content} (${item.status})`;
    });
    return { content: [{ type: "text", text: `${title}:\n${lines.join("\n")}` }] };
  };

  const tool: AgentTool<typeof inputSchema> = {
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
        items = tasks.map((content, idx) => ({ id: idx + 1, content, status: "pending" }));
        await publish("created");
        return render("Initialized task list");
      }

      if (op === "append") {
        if (!tasks || tasks.length === 0) {
          return {
            content: [
              {
                type: "text",
                text: "Error: 'append' operation requires a non-empty 'tasks' array.",
              },
            ],
            isError: true,
          };
        }
        const startId = items.length + 1;
        items.push(
          ...tasks.map((content, idx) => ({
            id: startId + idx,
            content,
            status: "pending" as const,
          })),
        );
        await publish("created");
        return render("Appended tasks");
      }

      if (op === "start" || op === "done") {
        if (index === undefined) {
          return {
            content: [{ type: "text", text: `Error: '${op}' operation requires task 'index'.` }],
            isError: true,
          };
        }
        const item = items.find((task) => task.id === index);
        if (!item) {
          return {
            content: [{ type: "text", text: `Error: Task index ${index} not found.` }],
            isError: true,
          };
        }
        item.status = op === "done" ? "completed" : "in_progress";
        await publish("updated");
        return render(`Marked task #${index} as ${item.status}`);
      }

      await publish("updated");
      return render("Current task list status");
    },
  };

  return {
    tool,
    getItems: () => items,
    clear: () => {
      items = [];
      void publish("updated");
    },
  };
}

const defaultController = createTodoTool();
export const todoTool = defaultController.tool;
export const getSessionTodoList = defaultController.getItems;
export const clearSessionTodoList = defaultController.clear;
