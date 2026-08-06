import type { AgentTool } from "@console/types";
import type { ToolSet } from "ai";
import { zodToJsonSchema } from "zod-to-json-schema";

export function convertOpencodeTools(tools: AgentTool[]): ToolSet {
  const result: ToolSet = {};

  for (const tool of tools) {
    result[tool.name] = {
      description: tool.description,
      parameters: zodToJsonSchema(tool.inputSchema, {
        target: "openApi3",
        $refStrategy: "none",
      }) as any,
    };
  }

  return result;
}
