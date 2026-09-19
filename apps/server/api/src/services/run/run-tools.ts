import { allTools } from "@/agent/src/tools/index.js";
import { createAskManyTool, createAskTool } from "@/agent/src/tools/ask.js";
import { createMemoryTool } from "@/agent/src/tools/memory.js";
import { createTodoTool, type TodoItem } from "@/agent/src/tools/todo.js";
import { findModelInProvider, inferThinkingLevels } from "@/agent/src/commands/provider-registry.js";
import type { AgentTool, AskQuestionRequest, Model, ProviderId } from "@console/types";
import { bindToolCwd } from "@console/types";

export function buildRunModel(provider: string, modelId: string): Model {
  const catalogModel = findModelInProvider(provider, modelId);
  // Fall back to naming-convention inference when the catalog hasn't
  // discovered this id yet, so it isn't wrongly treated as thinking-incapable.
  const thinking =
    catalogModel?.supportedThinkingLevels || catalogModel?.defaultThinkingLevel
      ? {
          ...(catalogModel.supportedThinkingLevels
            ? { supportedThinkingLevels: catalogModel.supportedThinkingLevels }
            : {}),
          ...(catalogModel.defaultThinkingLevel
            ? { defaultThinkingLevel: catalogModel.defaultThinkingLevel }
            : {}),
        }
      : inferThinkingLevels(provider as ProviderId, modelId);
  return {
    id: modelId,
    provider: provider as Model["provider"],
    contextWindow: catalogModel?.contextWindow ?? 128_000,
    ...(typeof catalogModel?.supportsImages === "boolean"
      ? { supportsImages: catalogModel.supportsImages }
      : {}),
    ...thinking,
  };
}

export interface AssembleToolsParams {
  cwd: string;
  projectId?: string | null;
  initialTodos: TodoItem[];
  askHandler: (request: AskQuestionRequest) => Promise<string | string[]>;
  onTodoUpdate: (items: TodoItem[], action: "created" | "updated") => void;
}

export function assembleAgentTools(params: AssembleToolsParams) {
  const tools = allTools.map((tool) => bindToolCwd(tool as AgentTool, params.cwd));

  const askTool = createAskTool(params.askHandler);
  const askManyTool = createAskManyTool(params.askHandler);
  const sessionTodo = createTodoTool(params.initialTodos, params.onTodoUpdate);
  const sessionMemory = createMemoryTool(params.projectId ?? null);

  const boundTools = tools.map((tool) => {
    if (tool.name === "ask") return askTool;
    if (tool.name === "askMany") return askManyTool;
    if (tool.name === "todo") return sessionTodo.tool;
    if (tool.name === "memory") return sessionMemory;
    return tool;
  });

  return boundTools;
}
