/**
 * Unit & Functional Tests for Permissions & Approval Engine.
 * Runs 100% offline — zero LLM calls / 0 credit usage.
 */
import assert from "node:assert/strict";
import {
  Agent,
  agentLoop,
  bashTool,
  readFileTool,
  resolveApproval,
  writeFileTool,
  type AgentTool,
  type Model,
  type StreamFn,
} from "@/agent/src/index.js";

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

  // write tool (plan-mode has full permissions)
  assert.equal(resolveApproval(writeFileTool, {}, "always-ask").policy, "prompt");
  assert.equal(resolveApproval(writeFileTool, {}, "accept-edits").policy, "allow");
  assert.equal(resolveApproval(writeFileTool, {}, "plan-mode").policy, "allow");
  assert.equal(resolveApproval(writeFileTool, {}, "full-access").policy, "allow");

  // exec tool (bash)
  assert.equal(resolveApproval(bashTool, {}, "always-ask").policy, "prompt");
  assert.equal(resolveApproval(bashTool, {}, "accept-edits").policy, "prompt");
  assert.equal(resolveApproval(bashTool, {}, "plan-mode").policy, "allow");
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
    approvalMode: "always-ask",
    onApproval: async (req) => {
      permissionEventReceived = true;
      assert.equal(req.toolName, "writeFile");
      return false; // Deny permission
    },
  });

  const events = [];
  for await (const event of stream) {
    events.push(event);
  }

  const result = await stream.result();
  assert.equal(permissionEventReceived, true);
  assert.equal(
    events.some((event) => event.type === "permissionRequest"),
    true,
  );
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

// 2b. Cancelling a pending permission produces a terminal tool result.
{
  const controller = new AbortController();
  let approvalRequested = false;
  const stream = agentLoop("Write file", {
    model: testModel,
    systemPrompt: "Test",
    tools: [writeFileTool as unknown as AgentTool],
    streamFn: async function* () {
      yield {
        type: "toolCall",
        id: "call_cancelled",
        name: "writeFile",
        argumentsJson: JSON.stringify({ path: "test.txt", content: "hello" }),
      };
    },
    approvalMode: "always-ask",
    onApproval: () => {
      approvalRequested = true;
      return new Promise<boolean>((_resolve, reject) => {
        queueMicrotask(() => {
          controller.abort();
          reject(new Error("Run aborted while waiting for permission."));
        });
      });
    },
    signal: controller.signal,
  });

  for await (const _event of stream) {
    // Consume the stream until the cancelled tool turn reaches sessionEnd.
  }
  const result = await stream.result();
  assert.equal(approvalRequested, true);
  assert.equal(result[2]?.role, "toolResult");
  if (result[2]?.role === "toolResult") {
    assert.equal(result[2].results[0]?.toolCallId, "call_cancelled");
    assert.equal(result[2].results[0]?.isError, true);
  }
  console.log("  ✅ cancelled permission produces terminal tool result");
}

// 3. Test Plan Mode system prompt injection
{
  const { buildSystemPrompt } = await import("@/agent/src/systemprompt/index.js");
  const promptRes = await buildSystemPrompt({
    approvalMode: "plan-mode",
  });

  assert.ok(promptRes.systemPrompt.includes("# Approval Mode Active"));
  assert.ok(promptRes.systemPrompt.includes("FULL permissions to explore and act"));
  assert.ok(promptRes.systemPrompt.includes("You MAY write, edit, and execute commands"));
  console.log("  ✅ Plan Mode system prompt injection");
}

console.log("Permissions & Approval Engine tests passed!\n");
