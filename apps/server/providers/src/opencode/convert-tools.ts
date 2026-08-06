/**
 * Converts AgentTool[] to the AI SDK ToolSet format for the opencode provider.
 * OpenAI accepts standard JSON Schema, so no CCA normalization is needed.
 */
import { zodToJsonSchema } from "zod-to-json-schema";
import type { AgentTool } from "@console/types";
import type { ToolSet } from "ai";

export function convertOpencodeTools(tools: AgentTool[]): ToolSet {
  const result: ToolSet = {};

  for (const tool of tools) {
    const rawSchema = zodToJsonSchema(tool.inputSchema, {
      target: "openApi3",
      $refStrategy: "none",
    }) as Record<string, unknown>;

    result[tool.name] = {
      description: tool.description,
      parameters: rawSchema,
    };
  }

  return result;
}
