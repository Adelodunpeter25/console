import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { writeFile } from "node:fs/promises";
import { z } from "zod";
import { Agent, type Model, type StreamFn } from "@/agent/src/index.js";
import type { AgentMessage } from "@console/types";

console.log("Running compaction lifecycle integration tests...");

// Isolate role settings: ambient machine config (e.g. a smol role) must not
// change which summary path these tests exercise.
const settingsDir = mkdtempSync(join(tmpdir(), "compaction-settings-"));
process.env.CONSOLE_SETTINGS_PATH = join(settingsDir, "settings.json");
await writeFile(process.env.CONSOLE_SETTINGS_PATH, JSON.stringify({ modelRoles: {} }));

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
  assert.equal((agentDefault as any)._compaction?.keepRecentTokens, 40_000);
  assert.equal((agentDefault as any)._compaction?.minimumRecentTurns, 3);
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

  // Structural summaries are the default strategy; LLM needs explicit opt-in.
  assert.equal((agentDefault as any)._compaction?.summaryStrategy, "structural");

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

// 3. Mid-turn shake fires after a bloated tool result
{
  let calls = 0;
  const mockStreamFn: StreamFn = async function* () {
    calls++;
    if (calls === 1) {
      yield { type: "toolCall", id: "c1", name: "read", argumentsJson: "" };
      yield { type: "toolCall", id: "c1", name: "read", argumentsJson: '{"path":"big.txt"}' };
    } else {
      yield { type: "text", text: "done" };
    }
  };

  const agent = new Agent({
    model: testModel,
    tools: [
      {
        name: "read",
        description: "read a file",
        inputSchema: z.object({ path: z.string() }),
        execute: async () => "x".repeat(100_000),
      },
    ],
    streamFn: mockStreamFn,
    approvalMode: "full-access",
    compaction: { enabled: true, tokenThreshold: 1_000 },
  });

  let midTurnEvent: any = null;
  const stream = agent.run("read the file");
  for await (const event of stream) {
    if (event.type === "compaction" && (event as any).trigger === "mid_turn") {
      midTurnEvent = event;
    }
  }

  assert.notEqual(midTurnEvent, null, "expected a mid_turn compaction event");
  assert.equal(midTurnEvent.tier, "shake");
  const stored = agent.messages.find((m) => m.role === "toolResult");
  assert.ok(stored && stored.role === "toolResult");
  const content = stored.results[0]!.content;
  assert.ok(typeof content === "string" && content.length < 20_000, "bloated result must be shaken mid-turn");
  console.log("  ✅ Mid-turn shake truncates bloated tool output before the next request");
}

// 4. Overflow error triggers emergency recovery + exactly one retry
{
  let calls = 0;
  const mockStreamFn: StreamFn = async function* (): AsyncGenerator<any> {
    calls++;
    if (calls === 1) {
      throw new Error("400 Bad Request: input token count exceeds 1048576");
    }
    yield { type: "text", text: "recovered" };
  };

  const agent = new Agent({ model: testModel, tools: [], streamFn: mockStreamFn });

  let recoveryEvent: any = null;
  let text = "";
  const stream = agent.run("hello");
  for await (const event of stream) {
    if (event.type === "compaction" && (event as any).trigger === "overflow_retry") {
      recoveryEvent = event;
    }
    if (event.type === "modelStreamPart" && typeof (event as any).part?.text === "string") {
      text += (event as any).part.text;
    }
  }

  assert.equal(calls, 2, "overflow must retry the turn exactly once");
  assert.notEqual(recoveryEvent, null, "expected an overflow_retry compaction event");
  assert.equal(recoveryEvent.tier, "emergency_recovery");
  assert.ok(text.includes("recovered"));
  console.log("  ✅ Overflow error recovers with emergency compaction + one retry");
}

// 5. A second overflow ends the turn with a structured error, never a loop
{
  let calls = 0;
  const mockStreamFn: StreamFn = async function* (): AsyncGenerator<any> {
    calls++;
    throw new Error("context_length_exceeded: maximum context length reached");
  };

  const agent = new Agent({ model: testModel, tools: [], streamFn: mockStreamFn });

  let errorEvent: any = null;
  const stream = agent.run("hello");
  for await (const event of stream) {
    if (event.type === "error") errorEvent = event;
  }

  assert.equal(calls, 2, "must not retry more than once");
  assert.notEqual(errorEvent, null, "expected a terminal error event");
  assert.ok(/context_length_exceeded/.test(errorEvent.error.message));
  console.log("  ✅ Repeated overflow terminates with a structured error");
}

// 6. Repeat read after truncation returns a diagnostic without re-executing
{
  let executions = 0;
  let calls = 0;
  const mockStreamFn: StreamFn = async function* () {
    calls++;
    if (calls <= 2) {
      yield { type: "toolCall", id: `c${calls}`, name: "read", argumentsJson: "" };
      yield {
        type: "toolCall",
        id: `c${calls}`,
        name: "read",
        argumentsJson: '{"path":"same.txt"}',
      };
    } else {
      yield { type: "text", text: "moving on" };
    }
  };

  const agent = new Agent({
    model: testModel,
    tools: [
      {
        name: "read",
        description: "read a file",
        inputSchema: z.object({ path: z.string() }),
        execute: async () => {
          executions++;
          return "y".repeat(50_000);
        },
      },
    ],
    streamFn: mockStreamFn,
    approvalMode: "full-access",
    compaction: { enabled: true, tokenThreshold: 1_000_000 },
  });

  const stream = agent.run("read the same file twice");
  for await (const _ of stream) {
    // drain
  }

  assert.equal(executions, 1, "truncated repeat must not re-execute");
  const toolResults = agent.messages.filter((m) => m.role === "toolResult");
  assert.equal(toolResults.length, 2);
  const second = toolResults[1]!;
  assert.ok(second.role === "toolResult");
  assert.ok((second.results[0]!.content as string).includes("already returned a truncated result"));
  console.log("  ✅ Truncated repeat returns a diagnostic instead of re-reading");
}

// 7. Identical requests are capped per run with a stop diagnostic
{
  let executions = 0;
  let calls = 0;
  const mockStreamFn: StreamFn = async function* () {
    calls++;
    if (calls <= 4) {
      yield { type: "toolCall", id: `d${calls}`, name: "read", argumentsJson: "" };
      yield {
        type: "toolCall",
        id: `d${calls}`,
        name: "read",
        argumentsJson: '{"path":"other.txt"}',
      };
    } else {
      yield { type: "text", text: "fine" };
    }
  };

  const agent = new Agent({
    model: testModel,
    tools: [
      {
        name: "read",
        description: "read a file",
        inputSchema: z.object({ path: z.string() }),
        execute: async () => {
          executions++;
          return "small result";
        },
      },
    ],
    streamFn: mockStreamFn,
    approvalMode: "full-access",
    compaction: { enabled: true, tokenThreshold: 1_000_000 },
  });

  const stream = agent.run("read in a loop");
  for await (const _ of stream) {
    // drain
  }

  assert.equal(executions, 3, "identical requests stop after 3 executions");
  const toolResults = agent.messages.filter((m) => m.role === "toolResult");
  const last = toolResults[toolResults.length - 1]!;
  assert.ok(last.role === "toolResult");
  assert.ok((last.results[0]!.content as string).includes("Stopping:"));
  console.log("  ✅ Per-run repeat cap terminates loops with a diagnostic");
}

// 8. Configured smol role takes the LLM summary path
{
  await writeFile(
    process.env.CONSOLE_SETTINGS_PATH!,
    JSON.stringify({ modelRoles: { smol: "antigravity/claude-opus-4-6-thinking" } }),
  );
  try {
    const mockStreamFn: StreamFn = async function* () {
      yield { type: "text", text: "smol condensed history" };
    };

    const agent = new Agent({
      model: testModel,
      tools: [],
      streamFn: mockStreamFn,
      compaction: { enabled: true, summaryStrategy: "llm", tokenThreshold: 100, keepRecentTokens: 50 },
    });

    const priorHistory: AgentMessage[] = [];
    for (let i = 1; i <= 5; i++) {
      priorHistory.push({ role: "user", content: `User request ${i}: refactor the auth module.` });
      priorHistory.push({
        role: "assistant",
        id: `s_${i}`,
        content: [{ type: "text", text: `Assistant response ${i}: done. `.repeat(10) }],
        stopReason: "stop",
      });
    }
    agent.loadHistory(priorHistory);

    let compactionEvent: any = null;
    const stream = agent.run("next turn");
    for await (const event of stream) {
      if (event.type === "compaction") compactionEvent = event;
    }

    assert.notEqual(compactionEvent, null);
    assert.ok(
      (compactionEvent.summary as string).includes("smol condensed history"),
      "smol summary must replace the structural checkpoint",
    );
    assert.ok(
      !(compactionEvent.summary as string).includes("<summary>"),
      "structural checkpoint must not win when smol is configured",
    );
    console.log("  ✅ Configured smol role produces LLM compaction summaries");
  } finally {
    await writeFile(process.env.CONSOLE_SETTINGS_PATH!, JSON.stringify({ modelRoles: {} }));
  }
}

console.log("All compaction lifecycle integration tests passed! ✨");
