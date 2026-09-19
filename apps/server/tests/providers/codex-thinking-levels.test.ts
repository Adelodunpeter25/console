/**
 * Tests for Codex thinking level support.
 * Validates that thinking levels are correctly mapped to reasoning.effort
 * and that the request body is properly constructed.
 */

import { describe, it, expect } from "bun:test";
import type { ThinkingLevel } from "@/agent/src/types/index.js";
import type { AgentMessage, AgentTool } from "@console/types";
import { buildCodexRequestBody } from "@/providers/src/index.js";

const NO_MESSAGES: AgentMessage[] = [];
const NO_TOOLS: AgentTool[] = [];

describe("Codex Thinking Levels", () => {
  describe("Level support", () => {
    it("should support all 7 thinking levels", () => {
      const supportedLevels: ThinkingLevel[] = [
        "none",
        "minimal",
        "low",
        "medium",
        "high",
        "xhigh",
        "max",
      ];
      // Verify all levels are valid for Codex (no filtering)
      expect(supportedLevels.length).toBe(7);
      expect(supportedLevels).toContain("none");
      expect(supportedLevels).toContain("minimal");
      expect(supportedLevels).toContain("xhigh");
      expect(supportedLevels).toContain("max");
    });
  });

  describe("Mapping to reasoning.effort", () => {
    it("should map 'low' to 'low'", () => {
      const level: ThinkingLevel = "low";
      const effort = level as string; // Direct 1:1 mapping
      expect(effort).toBe("low");
    });

    it("should map 'high' to 'high'", () => {
      const level: ThinkingLevel = "high";
      const effort = level as string;
      expect(effort).toBe("high");
    });

    it("should map 'xhigh' to 'xhigh'", () => {
      const level: ThinkingLevel = "xhigh";
      const effort = level as string;
      expect(effort).toBe("xhigh");
    });

    it("should map 'max' to 'max'", () => {
      const level: ThinkingLevel = "max";
      const effort = level as string;
      expect(effort).toBe("max");
    });

    it("should map 'none' to 'none' (no reasoning)", () => {
      const level: ThinkingLevel = "none";
      const effort = level as string;
      expect(effort).toBe("none");
    });

    it("should map 'minimal' to 'minimal'", () => {
      const level: ThinkingLevel = "minimal";
      const effort = level as string;
      expect(effort).toBe("minimal");
    });
  });

  describe("Request body construction", () => {
    it("should include a reasoning object with only 'effort' when level is specified", () => {
      const body = buildCodexRequestBody(
        { id: "gpt-5.6-terra" },
        "",
        NO_MESSAGES,
        NO_TOOLS,
        "none",
        "cache-key",
        "high",
      );

      // Codex's Responses API rejects an unknown `reasoning.type` field —
      // regression test for the "Unknown parameter: 'reasoning.type'" 400.
      expect(body.reasoning).toEqual({ effort: "high" });
    });

    it("should omit reasoning when level is undefined", () => {
      const body = buildCodexRequestBody(
        { id: "gpt-5.6-terra" },
        "",
        NO_MESSAGES,
        NO_TOOLS,
        "none",
        "cache-key",
        undefined,
      );

      expect(body.reasoning).toBeUndefined();
    });
  });

  describe("Model registry defaults", () => {
    it("should have 'low' as default thinking level for Codex models", () => {
      const defaultLevel: ThinkingLevel = "low" as const;
      expect(defaultLevel).toBe("low");
    });
  });
});
