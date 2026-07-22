/**
 * Converts AgentTool[] to Gemini functionDeclarations[] wire format.
 * Uses zod-to-json-schema to convert Zod schemas to JSON Schema.
 */
import { zodToJsonSchema } from "zod-to-json-schema";
import type { AgentTool } from "../../../agent/src/types/index.js";
import type { GeminiFunctionDeclaration } from "../types/index.js";

/**
 * Simplified CCA schema normalization based on oh-my-pi reference.
 *
 * Key differences from simple sanitization:
 * - Handles combiners (anyOf, oneOf) by merging or collapsing
 * - Converts type arrays to nullable for CCA compatibility
 * - Normalizes field names (snake_case to camelCase)
 * - Strips combiners that CCA doesn't support
 * - Only strips banned JSON Schema keywords at schema nodes; property
 *   names under `properties` (e.g. a tool arg named `pattern`) are kept
 */
function normalizeSchemaForCCA(schema: Record<string, unknown>): Record<string, unknown> {
  const banned = new Set([
    "$schema",
    "$ref",
    "$defs",
    "$dynamicRef",
    "$dynamicAnchor",
    "exclusiveMinimum",
    "exclusiveMaximum",
    "minimum",
    "maximum",
    "multipleOf",
    "minLength",
    "maxLength",
    "minItems",
    "maxItems",
    "pattern",
    "format",
    "minProperties",
    "maxProperties",
    "patternProperties",
    "propertyNames",
    "unevaluatedProperties",
    "unevaluatedItems",
    "dependencies",
    "dependentSchemas",
    "dependentRequired",
    "additionalProperties",
    "examples",
    "prefixItems",
  ]);

  // Object keys that map user-defined names -> nested schemas.
  // Keys inside these maps are property/definition names, not keywords.
  const propertyMapKeys = new Set(["properties", "definitions", "$defs", "dependentSchemas"]);

  const snakeToCamelMap = new Map([
    ["additional_properties", "additionalProperties"],
    ["any_of", "anyOf"],
    ["one_of", "oneOf"],
    ["prefix_items", "prefixItems"],
  ]);

  function normalize(value: unknown, isPropertyMap = false): unknown {
    if (Array.isArray(value)) {
      return value.map((item) => normalize(item, false));
    }
    if (typeof value !== "object" || value === null) {
      return value;
    }

    const obj = value as Record<string, unknown>;
    const result: Record<string, unknown> = {};

    for (const [key, val] of Object.entries(obj)) {
      const normalizedKey = snakeToCamelMap.get(key) ?? key;

      // Strip banned keywords only at schema level, never as property names
      // (e.g. grep/glob tools require a field literally named `pattern`)
      if (!isPropertyMap && banned.has(normalizedKey)) continue;

      // Handle combiners - CCA doesn't support anyOf/oneOf well
      if (!isPropertyMap && (normalizedKey === "anyOf" || normalizedKey === "oneOf")) {
        const variants = val as unknown[];
        if (Array.isArray(variants) && variants.length > 0) {
          // Simple fallback: use the first variant if all are objects
          const objectVariants = variants.filter(
            (v): v is Record<string, unknown> => typeof v === "object" && v !== null,
          );
          if (objectVariants.length > 0) {
            // Merge properties from all object variants
            const merged: Record<string, unknown> = { type: "object" };
            for (const variant of objectVariants) {
              if (variant.type === "object" && typeof variant.properties === "object") {
                Object.assign((merged.properties = merged.properties ?? {}), variant.properties);
              }
            }
            Object.assign(result, normalize(merged, false) as Record<string, unknown>);
            continue;
          }
        }
      }

      // Handle type arrays - convert to nullable
      if (!isPropertyMap && normalizedKey === "type" && Array.isArray(val)) {
        const types = val as string[];
        const nonNull = types.filter((t) => t !== "null");
        if (nonNull.length === 1) {
          result.type = nonNull[0];
          if (types.includes("null")) {
            result.nullable = true;
          }
          continue;
        }
      }

      const nextIsPropertyMap = !isPropertyMap && propertyMapKeys.has(normalizedKey);
      result[normalizedKey] = normalize(val, nextIsPropertyMap);
    }

    // Drop required entries for properties that no longer exist
    if (
      !isPropertyMap &&
      Array.isArray(result.required) &&
      typeof result.properties === "object" &&
      result.properties !== null
    ) {
      const propKeys = new Set(Object.keys(result.properties as object));
      result.required = (result.required as unknown[]).filter(
        (name) => typeof name === "string" && propKeys.has(name),
      );
      if ((result.required as unknown[]).length === 0) {
        delete result.required;
      }
    }

    return result;
  }

  return normalize(schema) as Record<string, unknown>;
}

export function convertTools(tools: AgentTool[], _modelId?: string): GeminiFunctionDeclaration[] {
  return tools.map((tool) => {
    const rawSchema = zodToJsonSchema(tool.inputSchema, {
      target: "openApi3",
      $refStrategy: "none",
    }) as Record<string, unknown>;

    // zodToJsonSchema wraps in { type: "object", properties, required, ... }
    // CCA wants parameters at this level
    const parameters = normalizeSchemaForCCA(rawSchema);

    // Claude models need 'parameters' field, others use 'parameters' as well
    // (the reference uses 'parameters' for CCA, not 'parametersJsonSchema')
    const declaration: GeminiFunctionDeclaration = {
      name: tool.name,
      description: tool.description,
      parameters,
    };

    return declaration;
  });
}
