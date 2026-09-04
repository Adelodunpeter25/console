import assert from "node:assert/strict";
import { Agent, type Model, type StreamFn } from "@/agent/src/index.js";
import type { AgentMessage } from "@console/types";

console.log("Running compaction lifecycle integration tests...");

const testModel: Model = {
  id: "test-model",
  provider: "antigravity",
  contextWindow: 128_000,
};

// 1. Agent options default configuration
{
  const mockStreamFn: StreamFn = async function* () {
    yield { type: "text", text: "Response" };
  };

  const agentDefault = new Agent({
    model: testModel,
    tools: [],
    streamFn: mockStreamFn,
  });

  // Verify internal compaction config exists and defaults match
  assert.equal((agentDefault as any)._compaction?.enabled, true);
  assert.equal((agentDefault as any)._compaction?.keepRecentTokens, 20_000);
  assert.equal((agentDefault as any)._compaction?.maxThresholdRatio, 0.85);
  assert.equal((agentDefault as any)._compaction?.maxToolResultChars, 8_000);

  // When explicitly passed false, compaction is disabled
  const agentDisabled = new Agent({
    model: testModel,
    tools: [],
    streamFn: mockStreamFn,
    compaction: false,
  });
  assert.equal((agentDisabled as any)._compaction, undefined);

  console.log("  ✅ Agent initializes default compaction and supports explicit opt-out");
}

// 2. Compaction trigger during Agent.run() when threshold is exceeded
{
  const mockStreamFn: StreamFn = async function* () {
    yield { type: "text", text: "Answer to current turn" };
  };

  // Set a small tokenThreshold (e.g. 100 tokens) so compaction triggers immediately
  const agent = new Agent({
    model: testModel,
    tools: [],
    streamFn: mockStreamFn,
    compaction: {
      enabled: true,
      tokenThreshold: 100,
      keepRecentTokens: 50,
    },
  });

  // Preload a substantial multi-turn history into the agent
  const priorHistory: AgentMessage[] = [];
  for (let i = 1; i <= 6; i++) {
    priorHistory.push({
      role: "user",
      content: `User request ${i}: Please refactor database queries and handle errors thoroughly.`,
    });
    priorHistory.push({
      role: "assistant",
      id: `a_${i}`,
      content: [
        {
          type: "text",
          text: `Assistant response ${i}: Executed query updates and verified schema integrity. `.repeat(10),
        },
      ],
      stopReason: "stop",
    });
  }

  agent.loadHistory(priorHistory);
  assert.equal(agent.messages.length, 12);

  let compactionEvent: any = null;

  const stream = agent.run("New user prompt for next turn");
  for await (const event of stream) {
    if (event.type === "compaction") {
      compactionEvent = event;
    }
  }

  // Verify compaction event was emitted with enriched payload
  assert.notEqual(compactionEvent, null);
  assert.equal(compactionEvent.type, "compaction");
  assert.equal(typeof compactionEvent.summary, "string");
  assert.equal(compactionEvent.summary.includes("<summary>"), true);
  assert.equal(compactionEvent.originalMessageCount >= 12, true);
  assert.equal(Array.isArray(compactionEvent.compactedMessages), true);
  assert.equal(compactionEvent.compactedMessages.length < 12, true);

  // Verify agent.messages reflects the compacted history + new turn
  assert.equal(agent.messages.length < priorHistory.length + 2, true);
  assert.equal(agent.messages[0].role, "user");
  assert.equal(agent.messages[1].role, "assistant");

  console.log("  ✅ Compaction triggers during run(), emits enriched event, and syncs agent.messages");
}

console.log("All compaction lifecycle integration tests passed! ✨");
