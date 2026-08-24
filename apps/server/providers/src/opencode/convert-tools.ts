import type { AgentTool } from "@console/types";
import { tool, type ToolSet } from "ai";

/**
 * Converts AgentTool[] to an AI SDK ToolSet for streamText.
 *
 * Uses the SDK's own `tool()` helper with the original Zod schemas so the SDK
 * owns all schema conversion. Passing raw JSON Schema under `parameters:` (the
 * AI SDK v4 convention) breaks on ai@7: streamText reads `tool.inputSchema`,
 * finds it missing, and `asSchema(undefined)` substitutes
 * `{ properties: {}, additionalProperties: false }` — leaving the model no
 * known parameters and making every parameterized call fail validation.
 */
export function convertOpencodeTools(tools: AgentTool[]): ToolSet {
  const result: ToolSet = {};

  for (const t of tools) {
    result[t.name] = tool({
      description: t.description,
      inputSchema: t.inputSchema,
    });
  }

  return result;
}
