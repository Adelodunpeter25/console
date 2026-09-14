/**
 * Unit Tests for the built-in `/init` slash command.
 * Zero network/API calls — matcher + system prompt assembly only.
 */
import assert from "node:assert/strict";
import {
  INIT_COMMAND_DESCRIPTION,
  INIT_COMMAND_DETAILS,
  INIT_COMMAND_NAME,
  isInitPrompt,
} from "@/agent/src/commands/init.js";
import { buildSystemPrompt } from "@/agent/src/systemprompt/index.js";

console.log("Running init command tests...");

// 1. Single builtin identity
{
  assert.equal(INIT_COMMAND_NAME, "init");
  assert.ok(INIT_COMMAND_DESCRIPTION.length > 0);
  assert.ok(INIT_COMMAND_DETAILS.includes("[scripts.<id>]"));
  console.log("  ✅ /init is the only builtin command identity");
}

// 2. Prompt matcher
{
  assert.ok(isInitPrompt("/init"));
  assert.ok(isInitPrompt("/init "));
  assert.ok(isInitPrompt("  /init generate scripts"));
  assert.ok(!isInitPrompt("/initialize"));
  assert.ok(!isInitPrompt("/Init"));
  assert.ok(!isInitPrompt("run /init"));
  assert.ok(!isInitPrompt(""));
  console.log("  ✅ /init prompt matcher");
}

// 3. System prompt always lists /init with the console.toml hint
{
  const result = await buildSystemPrompt({
    includeWorkspaceTree: false,
    preloaded: {
      commands: [],
      skills: [],
      rules: [],
      contextFiles: [],
    },
  });
  assert.ok(result.systemPrompt.includes("/init"));
  assert.ok(result.systemPrompt.includes("console.toml"));
  console.log("  ✅ System prompt carries the /init hint");
}

console.log("Init command tests passed!\n");
