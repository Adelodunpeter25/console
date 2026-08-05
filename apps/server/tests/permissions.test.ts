/**
 * Unit & Functional Tests for Permissions & Approval Engine.
 * Runs 100% offline — zero LLM calls / 0 credit usage.
 */
import assert from "node:assert/strict";
import {
  Agent,
  SlashCommandRegistry,
  agentLoop,
  bashTool,
  readFileTool,
  resolveApproval,
  writeFileTool,
  type AgentTool,
  type Model,
  type StreamFn,
} from "../agent/src/index.js";

console.log("Running Permissions & Approval Engine tests...");

const testModel: Model = {
  id: "test-model",
  provider: "antigravity",
  contextWindow: 128_000,
};

// 1. Test resolveApproval policy mapping across security modes
{
  // read tool is allowed in all modes
  assert.equal(resolveApproval(readFileTool, {}, "always-ask").policy, "allow");
  assert.equal(resolveApproval(readFileTool, {}, "accept-edits").policy, "allow");
  assert.equal(resolveApproval(readFileTool, {}, "plan-mode").policy, "allow");
  assert.equal(resolveApproval(readFileTool, {}, "full-access").policy, "allow");

  // write tool
  assert.equal(resolveApproval(writeFileTool, {}, "always-ask").policy, "prompt");
  assert.equal(resolveApproval(writeFileTool, {}, "accept-edits").policy, "allow");
  assert.equal(resolveApproval(writeFileTool, {}, "plan-mode").policy, "prompt");
  assert.equal(resolveApproval(writeFileTool, {}, "full-access").policy, "allow");

  // exec tool (bash)
  assert.equal(resolveApproval(bashTool, {}, "always-ask").policy, "prompt");
  assert.equal(resolveApproval(bashTool, {}, "accept-edits").policy, "prompt");
  assert.equal(resolveApproval(bashTool, {}, "plan-mode").policy, "prompt");
  assert.equal(resolveApproval(bashTool, {}, "full-access").policy, "allow");
  console.log("  ✅ resolveApproval mode policy mapping");
}

// 2. Test AgentLoop permission approval prompt & denial handling
{
  let streamCount = 0;
  const mockStreamFn: StreamFn = async function* () {
    streamCount += 1;
    if (streamCount === 1) {
      yield {
        type: "toolCall",
        id: "call_write",
        name: "writeFile",
        argumentsJson: JSON.stringify({ path: "test.txt", content: "hello" }),
      };
    } else {
      yield { type: "text", text: "Denied by user." };
    }
  };

  let permissionEventReceived = false;

  const stream = agentLoop("Write file", {
    model: testModel,
    systemPrompt: "Test",
    tools: [writeFileTool as unknown as AgentTool],
    streamFn: mockStreamFn,
    approvalMode: "plan-mode",
    onApproval: async (req) => {
      permissionEventReceived = true;
      assert.equal(req.toolName, "writeFile");
      assert.equal(req.requiresUpgrade, true);
      assert.ok(req.reason?.includes("upgraded permission"));
      return false; // Deny permission
    },
  });

  const events = [];
  for await (const event of stream) {
    events.push(event);
  }

  const result = await stream.result();
  assert.equal(permissionEventReceived, true);
  assert.equal(events.some((event) => event.type === "permissionRequest"), true);
  // History has user message, assistant toolCall, toolResult with isError
  assert.equal(result.length, 4);
  assert.equal(result[2]?.role, "toolResult");
  if (result[2]?.role === "toolResult") {
    assert.equal(result[2].results[0]?.isError, true);
    assert.ok(
      String(result[2].results[0]?.content).includes(
        "Execution denied by user permission decision",
      ),
    );
  }
  console.log("  ✅ agentLoop permissionRequest event & user denial handling");
}

// 3. Test /mode slash command
{
  const agent = new Agent({
    model: testModel,
    tools: [],
    systemPrompt: "Test",
    streamFn: async function* () {},
    approvalMode: "always-ask",
  });

  const registry = new SlashCommandRegistry();
  const ctx: any = { agent, currentProvider: "antigravity" };

  // List modes
  const listRes = await registry.parseAndExecute("/mode", ctx);
  assert.equal(listRes.handled, true);
  assert.ok(listRes.message?.includes("Current mode: always-ask"));

  // Switch to accept-edits
  const switchRes = await registry.parseAndExecute("/mode accept-edits", ctx);
  assert.equal(switchRes.handled, true);
  assert.equal(agent.approvalMode, "accept-edits");
  console.log("  ✅ /mode slash command");
}

// 4. Test Plan Mode system prompt injection
{
  const { buildSystemPrompt } = await import("../agent/src/systemprompt/index.js");
  const promptRes = await buildSystemPrompt({
    approvalMode: "plan-mode",
  });

  assert.ok(promptRes.systemPrompt.includes("# Approval Mode Active"));
  assert.ok(promptRes.systemPrompt.includes("READ-ONLY exploration and research"));
  assert.ok(promptRes.systemPrompt.includes("You MUST NOT write, edit, or delete any files"));
  console.log("  ✅ Plan Mode system prompt injection");
}

console.log("Permissions & Approval Engine tests passed!\n");
