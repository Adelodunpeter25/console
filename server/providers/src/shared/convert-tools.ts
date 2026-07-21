/**
 * Converts AgentTool[] to Gemini functionDeclarations[] wire format.
 * Uses zod-to-json-schema to convert Zod schemas to JSON Schema.
 */
import { zodToJsonSchema } from "zod-to-json-schema";
import type { AgentTool } from "../../../agent/src/types/index.js";
import type { GeminiFunctionDeclaration } from "../types/index.js";

/**
 * Strips JSON Schema keywords that Google Cloud Code Assist does not accept.
 *
 * `zod-to-json-schema` emits standard JSON Schema (Draft 2020-12) keywords that
 * the CCA function-declaration format rejects — it only accepts a narrow
 * OpenAPI 3.0 subset. The full list comes from Google's own schema validation:
 *   https://ai.google.dev/api/generate-content#v1beta.WriteToolConfig
 *
 * Key categories stripped here:
 *   - Meta/reference: $schema, $ref, $defs, $dynamicRef, $dynamicAnchor
 *   - Validation constraints: exclusiveMinimum, exclusiveMaximum, minimum,
 *     maximum, multipleOf, minLength, maxLength, minItems, maxItems, pattern,
 *     format, minProperties, maxProperties
 *   - Structural/advanced: patternProperties, propertyNames,
 *     unevaluatedProperties, unevaluatedItems, dependencies,
 *     dependentSchemas, dependentRequired, examples, prefixItems
 *   - additionalProperties (already not accepted by some Gemini models)
 */
function sanitizeSchema(schema: Record<string, unknown>): Record<string, unknown> {
  const banned = new Set([
    "$schema", "$ref", "$defs", "$dynamicRef", "$dynamicAnchor",
    "exclusiveMinimum", "exclusiveMaximum", "minimum", "maximum",
    "multipleOf", "minLength", "maxLength", "minItems", "maxItems",
    "pattern", "format", "minProperties", "maxProperties",
    "patternProperties", "propertyNames",
    "unevaluatedProperties", "unevaluatedItems",
    "dependencies", "dependentSchemas", "dependentRequired",
    "additionalProperties", "examples", "prefixItems",
  ]);
  const result: Record<string, unknown> = {};

  for (const [key, value] of Object.entries(schema)) {
    if (banned.has(key)) continue;

    if (key === "properties" && typeof value === "object" && value !== null) {
      result[key] = Object.fromEntries(
        Object.entries(value as Record<string, unknown>).map(([k, v]) => [
          k,
          typeof v === "object" && v !== null ? sanitizeSchema(v as Record<string, unknown>) : v,
        ]),
      );
    } else if (Array.isArray(value)) {
      result[key] = value.map((item) =>
        typeof item === "object" && item !== null
          ? sanitizeSchema(item as Record<string, unknown>)
          : item,
      );
    } else if (typeof value === "object" && value !== null) {
      result[key] = sanitizeSchema(value as Record<string, unknown>);
    } else {
      result[key] = value;
    }
  }

  return result;
}

export function convertTools(tools: AgentTool[]): GeminiFunctionDeclaration[] {
  return tools.map((tool) => {
    const rawSchema = zodToJsonSchema(tool.inputSchema, {
      target: "openApi3",
      $refStrategy: "none",
    }) as Record<string, unknown>;

    // zodToJsonSchema wraps in { type: "object", properties, required, ... }
    // Gemini wants parameters at this level
    const parameters = sanitizeSchema(rawSchema);

    return {
      name: tool.name,
      description: tool.description,
      parameters,
    };
  });
}
