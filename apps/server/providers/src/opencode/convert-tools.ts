import type { AgentTool } from "@console/types";
import type { ToolSet } from "ai";

export function convertOpencodeTools(tools: AgentTool[]): ToolSet {
  const result: ToolSet = {};

  for (const tool of tools) {
    result[tool.name] = {
      description: tool.description,
      inputSchema: tool.inputSchema,
    };
  }

  return result;
}
