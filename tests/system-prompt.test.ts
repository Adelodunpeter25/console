/**
 * Unit Tests for SystemPromptBuilder & Context Discovery.
 * Zero network/API calls — local filesystem & configuration discovery only.
 */
import assert from "node:assert/strict";
import { SystemPromptBuilder, buildSystemPrompt } from "../server/agent/src/systemprompt/index.js";

console.log("Running SystemPromptBuilder tests...");

// 1. Basic buildSystemPrompt assembly
{
  const result = await buildSystemPrompt({
    model: "gemini-3.1-pro",
    toolNames: ["readFile", "writeFile", "bash"],
    appendPrompt: "Note: test mode active.",
  });

  assert.ok(typeof result.systemPrompt === "string");
  assert.ok(result.systemPrompt.length > 0);
  assert.ok(result.systemPrompt.includes("Workstation"));
  assert.ok(result.systemPrompt.includes("`readFile`"));
  assert.ok(result.systemPrompt.includes("Note: test mode active."));
  assert.ok(Array.isArray(result.sections));
  assert.ok(result.sections.length > 0);
  console.log("  ✅ Basic system prompt assembly");
}

// 2. Custom prompt override & tree toggle
{
  const customIdentity = "You are a custom test assistant.";
  const result = await buildSystemPrompt({
    customPrompt: customIdentity,
    includeWorkspaceTree: false,
  });

  assert.ok(result.systemPrompt.startsWith(customIdentity));
  assert.equal(result.context.workspaceTree.rendered, "");
  console.log("  ✅ Custom prompt override & workspace tree toggle");
}

// 3. SystemPromptBuilder instance API
{
  const builder = new SystemPromptBuilder({
    toolNames: ["glob", "grep"],
  });

  const context = await builder.discover();
  assert.ok(context.environment.date);
  assert.ok(context.environment.cwd);

  const buildRes = await builder.build({
    appendPrompt: "Extra instruction",
  });
  assert.ok(buildRes.systemPrompt.includes("`glob`"));
  assert.ok(buildRes.systemPrompt.includes("`grep`"));
  assert.ok(buildRes.systemPrompt.includes("Extra instruction"));
  console.log("  ✅ SystemPromptBuilder instance API");
}

console.log("SystemPromptBuilder tests passed!\n");
