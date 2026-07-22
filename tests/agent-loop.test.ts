/**
 * Unit & Functional Tests for AgentLoop & Agent class.
 * NO external LLM API calls — uses mock StreamFn.
 */
import assert from "node:assert/strict";
import {
  Agent,
  agentLoop,
  type AgentTool,
  type Model,
  type StreamFn,
} from "../server/agent/src/index.js";
import { z } from "zod";

console.log("Running AgentLoop service tests...");

const testModel: Model = {
  id: "mock-model",
  provider: "antigravity",
  contextWindow: 128_000,
};

const dummyTool: AgentTool = {
  name: "dummyCalc",
  description: "Add two numbers",
  inputSchema: z.object({
    a: z.number(),
    b: z.number(),
  }),
  execute: async (args) => {
    const { a, b } = args as { a: number; b: number };
    return { sum: a + b };
  },
};

// 1. Test basic single-turn text streaming
{
  const mockStreamFn: StreamFn = async function* () {
    yield { type: "text", text: "Hello " };
    yield { type: "text", text: "World!" };
  };

  const stream = agentLoop("Hi there", {
    model: testModel,
    systemPrompt: "You are helpful",
    tools: [],
    streamFn: mockStreamFn,
  });

  const events = [];
  for await (const event of stream) {
    events.push(event);
  }

  const result = await stream.result();
  assert.equal(result.length, 2); // User + Assistant
  assert.equal(result[0]?.role, "user");
  assert.equal(result[1]?.role, "assistant");
  if (result[1]?.role === "assistant") {
    assert.equal(result[1].stopReason, "stop");
    assert.equal(result[1].content[0]?.type, "text");
    if (result[1].content[0]?.type === "text") {
      assert.equal(result[1].content[0].text, "Hello World!");
    }
  }
  console.log("  ✅ Single-turn text streaming");
}

// 2. Test tool execution turn and loop continuation
{
  let callCount = 0;
  const mockStreamFn: StreamFn = async function* () {
    callCount += 1;
    if (callCount === 1) {
      // First turn: model emits tool call
      yield {
        type: "toolCall",
        id: "call_1",
        name: "dummyCalc",
        argumentsJson: JSON.stringify({ a: 10, b: 20 }),
      };
    } else {
      // Second turn: model receives tool result and outputs text
      yield { type: "text", text: "The answer is 30." };
    }
  };

  const executedTools: string[] = [];
  const stream = agentLoop("Calculate 10 + 20", {
    model: testModel,
    systemPrompt: "Math bot",
    tools: [dummyTool],
    streamFn: mockStreamFn,
    onToolCall: (call) => {
      executedTools.push(call.name);
    },
  });

  for await (const _ of stream) {
  }

  const result = await stream.result();
  assert.equal(callCount, 2);
  assert.equal(executedTools.length, 1);
  assert.equal(executedTools[0], "dummyCalc");

  // History should contain: user prompt, assistant toolCall, toolResult, assistant final text
  assert.equal(result.length, 4);
  assert.equal(result[0]?.role, "user");
  assert.equal(result[1]?.role, "assistant");
  assert.equal(result[2]?.role, "toolResult");
  assert.equal(result[3]?.role, "assistant");
  console.log("  ✅ Tool execution turn & loop continuation");
}

// 3. Test Agent stateful class wrapper
{
  const mockStreamFn: StreamFn = async function* () {
    yield { type: "text", text: "Response 1" };
  };

  const agent = new Agent({
    model: testModel,
    tools: [],
    systemPrompt: "Test",
    streamFn: mockStreamFn,
  });

  assert.equal(agent.isRunning, false);
  assert.equal(agent.messages.length, 0);

  const stream = agent.run("Prompt 1");
  assert.equal(agent.isRunning, true);

  for await (const _ of stream) {
  }
  await stream.result();

  assert.equal(agent.isRunning, false);
  assert.equal(agent.messages.length, 2);

  agent.clearHistory();
  assert.equal(agent.messages.length, 0);
  console.log("  ✅ Agent stateful class wrapper");
}

console.log("AgentLoop service tests passed!\n");
