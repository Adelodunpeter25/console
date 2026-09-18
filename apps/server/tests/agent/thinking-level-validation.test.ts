/**
 * Tests for thinking level validation.
 * Validates that the agent properly rejects unsupported thinking levels
 * and accepts supported ones for each provider/model.
 */

import { describe, it, expect } from "bun:test";
import { validateThinkingLevelForModel } from "@/agent/src/service/validate-thinking.js";
import type { Model, ThinkingLevel } from "@/agent/src/types/index.js";

describe("Thinking Level Validation", () => {
  describe("Codex models", () => {
    const codexModel: Model = {
      id: "gpt-5.5",
      provider: "codex",
      contextWindow: 272_000,
      supportsImages: true,
      supportedThinkingLevels: ["none", "minimal", "low", "medium", "high", "xhigh", "max"],
      defaultThinkingLevel: "medium",
    };

    it("should accept 'low' thinking level", () => {
      expect(() => validateThinkingLevelForModel(codexModel, "low")).not.toThrow();
    });

    it("should accept 'high' thinking level", () => {
      expect(() => validateThinkingLevelForModel(codexModel, "high")).not.toThrow();
    });

    it("should accept 'xhigh' thinking level", () => {
      expect(() => validateThinkingLevelForModel(codexModel, "xhigh")).not.toThrow();
    });

    it("should accept 'max' thinking level", () => {
      expect(() => validateThinkingLevelForModel(codexModel, "max")).not.toThrow();
    });

    it("should accept 'none' thinking level", () => {
      expect(() => validateThinkingLevelForModel(codexModel, "none")).not.toThrow();
    });

    it("should accept 'minimal' thinking level", () => {
      expect(() => validateThinkingLevelForModel(codexModel, "minimal")).not.toThrow();
    });

    it("should accept undefined (uses default)", () => {
      expect(() => validateThinkingLevelForModel(codexModel, undefined)).not.toThrow();
    });
  });

  describe("Claude models", () => {
    const claudeModel: Model = {
      id: "claude-opus-4-6",
      provider: "claude",
      contextWindow: 1_000_000,
      supportsImages: true,
      supportedThinkingLevels: ["low", "medium", "high", "xhigh", "max"],
      defaultThinkingLevel: "high",
    };

    it("should accept 'low' thinking level", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, "low")).not.toThrow();
    });

    it("should accept 'high' thinking level", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, "high")).not.toThrow();
    });

    it("should accept 'xhigh' thinking level", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, "xhigh")).not.toThrow();
    });

    it("should accept 'max' thinking level", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, "max")).not.toThrow();
    });

    it("should reject 'none' thinking level", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, "none")).toThrow(
        /does not support thinking level "none"/
      );
    });

    it("should reject 'minimal' thinking level", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, "minimal")).toThrow(
        /does not support thinking level "minimal"/
      );
    });

    it("should accept undefined (uses default)", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, undefined)).not.toThrow();
    });
  });

  describe("Gemini models", () => {
    const geminiModel: Model = {
      id: "gemini-3.1-pro",
      provider: "antigravity",
      contextWindow: 1_048_576,
      supportedThinkingLevels: ["minimal", "low", "medium", "high"],
      defaultThinkingLevel: "medium",
    };

    it("should accept 'low' thinking level", () => {
      expect(() => validateThinkingLevelForModel(geminiModel, "low")).not.toThrow();
    });

    it("should accept 'high' thinking level", () => {
      expect(() => validateThinkingLevelForModel(geminiModel, "high")).not.toThrow();
    });

    it("should reject 'xhigh' thinking level (not supported by Gemini)", () => {
      expect(() => validateThinkingLevelForModel(geminiModel, "xhigh")).toThrow(
        /does not support thinking level "xhigh"/
      );
    });

    it("should reject 'max' thinking level (not supported by Gemini)", () => {
      expect(() => validateThinkingLevelForModel(geminiModel, "max")).toThrow(
        /does not support thinking level "max"/
      );
    });

    it("should reject 'none' thinking level (not supported by Gemini)", () => {
      expect(() => validateThinkingLevelForModel(geminiModel, "none")).toThrow(
        /does not support thinking level "none"/
      );
    });
  });

  describe("Models without thinking level support", () => {
    const simpleModel: Model = {
      id: "gpt-3.5-turbo",
      provider: "antigravity",
      contextWindow: 16_384,
    };

    it("should accept undefined (no thinking level requested)", () => {
      expect(() => validateThinkingLevelForModel(simpleModel, undefined)).not.toThrow();
    });

    it("should reject any thinking level for models without support", () => {
      expect(() => validateThinkingLevelForModel(simpleModel, "low")).toThrow(
        /does not support thinking levels/
      );
    });

    it("should provide clear error message when model has no thinking support", () => {
      expect(() => validateThinkingLevelForModel(simpleModel, "high")).toThrow(
        /Only providers with built-in reasoning/
      );
    });
  });

  describe("Error messages", () => {
    const claudeModel: Model = {
      id: "claude-sonnet-4-6",
      provider: "claude",
      contextWindow: 1_000_000,
      supportedThinkingLevels: ["low", "medium", "high", "xhigh", "max"],
      defaultThinkingLevel: "high",
    };

    it("should include model ID and level in error message", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, "none")).toThrow(
        /claude-sonnet-4-6/
      );
      expect(() => validateThinkingLevelForModel(claudeModel, "none")).toThrow(/"none"/);
    });

    it("should list supported levels in error message", () => {
      expect(() => validateThinkingLevelForModel(claudeModel, "none")).toThrow(
        /low.*medium.*high.*xhigh.*max/
      );
    });
  });
});
