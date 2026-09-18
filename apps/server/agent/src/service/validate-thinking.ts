/**
 * Validate thinking level support for a given model.
 * Throws an error if the requested level is not supported by the model.
 */
import type { Model, ThinkingLevel } from "@/agent/src/types/index.js";

export function validateThinkingLevelForModel(model: Model, level?: ThinkingLevel): void {
  // Omitted level is always valid (uses model's default)
  if (!level) return;

  // If the model doesn't specify supported levels, it doesn't support thinking levels at all
  if (!model.supportedThinkingLevels || model.supportedThinkingLevels.length === 0) {
    throw new Error(
      `Model "${model.id}" does not support thinking levels. ` +
      `Only providers with built-in reasoning (Codex, Claude, Gemini) support this feature.`
    );
  }

  // Check if the requested level is in the supported list
  if (!model.supportedThinkingLevels.includes(level)) {
    throw new Error(
      `Model "${model.id}" does not support thinking level "${level}". ` +
      `Supported levels: ${model.supportedThinkingLevels.join(", ")}`
    );
  }
}
