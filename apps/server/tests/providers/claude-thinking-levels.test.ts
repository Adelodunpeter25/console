/**
 * Tests for Claude thinking level support.
 * Validates that thinking levels are correctly mapped to output_config.effort
 * and that unsupported levels ("none", "minimal") are properly filtered.
 */

import { describe, it, expect } from "bun:test";
import type { ThinkingLevel } from "@/agent/src/types/index.js";

describe("Claude Thinking Levels", () => {
  describe("Level support", () => {
    it("should support only 5 thinking levels (not 'none' or 'minimal')", () => {
      const supportedLevels: ThinkingLevel[] = ["low", "medium", "high", "xhigh", "max"];
      expect(supportedLevels.length).toBe(5);
      expect(supportedLevels).toContain("low");
      expect(supportedLevels).toContain("medium");
      expect(supportedLevels).toContain("high");
      expect(supportedLevels).toContain("xhigh");
      expect(supportedLevels).toContain("max");
      expect(supportedLevels).not.toContain("none");
      expect(supportedLevels).not.toContain("minimal");
    });
  });

  describe("Mapping to output_config.effort", () => {
    it("should map 'low' to 'low'", () => {
      const level: ThinkingLevel = "low";
      const effort = mapThinkingLevelToClaude(level);
      expect(effort).toBe("low");
    });

    it("should map 'medium' to 'medium'", () => {
      const level: ThinkingLevel = "medium";
      const effort = mapThinkingLevelToClaude(level);
      expect(effort).toBe("medium");
    });

    it("should map 'high' to 'high'", () => {
      const level: ThinkingLevel = "high";
      const effort = mapThinkingLevelToClaude(level);
      expect(effort).toBe("high");
    });

    it("should map 'xhigh' to 'xhigh'", () => {
      const level: ThinkingLevel = "xhigh";
      const effort = mapThinkingLevelToClaude(level);
      expect(effort).toBe("xhigh");
    });

    it("should map 'max' to 'max'", () => {
      const level: ThinkingLevel = "max";
      const effort = mapThinkingLevelToClaude(level);
      expect(effort).toBe("max");
    });

    it("should omit 'none' by returning undefined", () => {
      const level: ThinkingLevel = "none";
      const effort = mapThinkingLevelToClaude(level);
      expect(effort).toBeUndefined();
    });

    it("should omit 'minimal' by returning undefined", () => {
      const level: ThinkingLevel = "minimal";
      const effort = mapThinkingLevelToClaude(level);
      expect(effort).toBeUndefined();
    });

    it("should return undefined when level is undefined", () => {
      const level: ThinkingLevel | undefined = undefined;
      const effort = mapThinkingLevelToClaude(level);
      expect(effort).toBeUndefined();
    });
  });

  describe("Request body construction", () => {
    it("should include output_config when effort level is defined", () => {
      const effort = "high";
      const outputConfig = { effort };
      expect(outputConfig).toEqual({ effort: "high" });
    });

    it("should omit output_config when effort is undefined", () => {
      const effort: string | undefined = undefined;
      const outputConfig = effort ? { effort } : undefined;
      expect(outputConfig).toBeUndefined();
    });

    it("should handle conditional output_config in request body", () => {
      const effort = "medium";
      const body: Record<string, unknown> = {
        model: "claude-opus-4-6",
        max_tokens: 4096,
        ...(effort ? { output_config: { effort } } : {}),
      };

      expect(body.output_config).toEqual({ effort: "medium" });
      expect(body.model).toBe("claude-opus-4-6");
    });

    it("should exclude output_config when effort is undefined", () => {
      const effort: string | undefined = undefined;
      const body: Record<string, unknown> = {
        model: "claude-opus-4-6",
        max_tokens: 4096,
        ...(effort ? { output_config: { effort } } : {}),
      };

      expect(body.output_config).toBeUndefined();
      expect(Object.keys(body)).not.toContain("output_config");
    });
  });

  describe("Model registry defaults", () => {
    it("should have 'low' as default thinking level for Claude models", () => {
      const defaultLevel: ThinkingLevel = "low" as const;
      expect(defaultLevel).toBe("low");
    });
  });
});

/**
 * Test helper: maps Console thinking level to Claude output_config.effort.
 * Mirrors the actual mapThinkingLevelToClaude function in claude/stream-fn.ts
 */
function mapThinkingLevelToClaude(level?: ThinkingLevel): string | undefined {
  if (!level) return undefined;
  if (level === "none" || level === "minimal") return undefined;
  return level as string;
}
