/**
 * Converts AgentTool[] to OpenAI function tools wire format.
 * OpenAI accepts standard JSON Schema, so no CCA normalization is needed.
 */
import { zodToJsonSchema } from "zod-to-json-schema";
import type { AgentTool } from "../../../agent/src/types/index.js";

export interface OpenAIFunctionTool {
  type: "function";
  function: {
    name: string;
    description: string;
    parameters: Record<string, unknown>;
  };
}

export function convertOpencodeTools(tools: AgentTool[]): OpenAIFunctionTool[] {
  return tools.map((tool) => {
    const rawSchema = zodToJsonSchema(tool.inputSchema, {
      target: "openApi3",
      $refStrategy: "none",
    }) as Record<string, unknown>;

    return {
      type: "function",
      function: {
        name: tool.name,
        description: tool.description,
        parameters: rawSchema,
      },
    };
  });
}
