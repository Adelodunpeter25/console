/**
 * Unit & Integration Tests for Subagent Streaming Events.
 */
import assert from "node:assert/strict";
import { Agent, type AgentTool, type Model, type StreamFn, type AgentSessionEvent } from "@/agent/src/index.js";
import { z } from "zod";

console.log("Running Subagent streaming tests...");

const testModel: Model = {
  id: "mock-model",
  provider: "antigravity",
  contextWindow: 128_000,
};

const dummySearchTool: AgentTool = {
  name: "searchFiles",
  description: "Search files in project",
  inputSchema: z.object({
    pattern: z.string(),
  }),
  execute: async (args) => {
    const { pattern } = args as { pattern: string };
    return { matches: [`src/${pattern}.ts`, `tests/${pattern}.test.ts`] };
  },
};

// 1. Test subagent tool execution and event emission
{
  let callCount = 0;
  const mockStreamFn: StreamFn = async function* (params) {
    callCount++;
    const isSubagent = params.systemPrompt?.includes("specialized subagent");
    if (!isSubagent) {
      if (callCount === 1) {
        // Parent agent calls subagent tool
        yield {
          type: "toolCall",
          id: "parent_call_1",
          name: "subagent",
          argumentsJson: JSON.stringify({
            prompt: "Search for files matching user",
            name: "Inspector",
            role: "Codebase Inspector",
          }),
        };
      } else {
        // Parent agent concludes after subagent completes
        yield { type: "text", text: "Subagent completed research." };
      }
    } else {
      // Child subagent agentLoop
      if (params.messages.length === 1) {
        // Subagent executes searchFiles
        yield {
          type: "toolCall",
          id: "sub_tool_call_1",
          name: "searchFiles",
          argumentsJson: JSON.stringify({ pattern: "user" }),
        };
      } else {
        // Subagent concludes with markdown summary
        yield { type: "text", text: "Found 2 files: src/user.ts and tests/user.test.ts." };
      }
    }
  };

  const agent = new Agent({
    model: testModel,
    tools: [dummySearchTool, { name: "subagent", description: "subagent", inputSchema: z.object({}), execute: async () => ({}) }],
    streamFn: mockStreamFn,
    approvalMode: "accept-edits",
  });

  const emittedEvents: AgentSessionEvent[] = [];
  const stream = agent.run("Find user files with subagent");

  for await (const event of stream) {
    emittedEvents.push(event);
  }

  await stream.result();

  // Verify subagent lifecycle events were emitted
  const startEvent = emittedEvents.find((e) => e.type === "subagentStart");
  assert.ok(startEvent, "subagentStart event should be emitted");
  if (startEvent && startEvent.type === "subagentStart") {
    assert.equal(startEvent.name, "Inspector");
    assert.equal(startEvent.role, "Codebase Inspector");
    assert.equal(startEvent.prompt, "Search for files matching user");
    assert.equal(startEvent.parentToolCallId, "parent_call_1");
  }

  const activityEvents = emittedEvents.filter((e) => e.type === "subagentActivity");
  assert.ok(activityEvents.length >= 2, "subagentActivity events should be emitted for tool execution");

  const runningActivity = activityEvents.find((e) => e.type === "subagentActivity" && e.status === "running");
  assert.ok(runningActivity, "running subagentActivity event should be emitted");
  if (runningActivity && runningActivity.type === "subagentActivity") {
    assert.equal(runningActivity.toolName, "searchFiles");
  }

  const completedActivity = activityEvents.find((e) => e.type === "subagentActivity" && e.status === "completed");
  assert.ok(completedActivity, "completed subagentActivity event should be emitted");
  if (completedActivity && completedActivity.type === "subagentActivity") {
    assert.equal(completedActivity.toolName, "searchFiles");
  }

  const endEvent = emittedEvents.find((e) => e.type === "subagentEnd");
  assert.ok(endEvent, "subagentEnd event should be emitted");
  if (endEvent && endEvent.type === "subagentEnd") {
    assert.equal(endEvent.status, "completed");
    assert.ok(endEvent.summary?.includes("Found 2 files"), "Summary should contain subagent output");
    assert.ok(endEvent.totalTurns > 0, "Total turns should be > 0");
  }

  console.log("  ✅ Subagent streaming lifecycle and activity events");
}
