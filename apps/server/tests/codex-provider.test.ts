/**
 * Codex Responses provider tests.
 * Deterministic tests use mocked SSE; the live test is opt-in with CODEX_REAL_API=1.
 */
import assert from "node:assert/strict";
import { Agent, type AgentTool, type Model } from "@/agent/src/index.js";
import { listDirTool } from "@/agent/src/tools/list-dir.js";
import { bindToolCwd } from "@console/types";
import { codexCredentialExists } from "@/providers/src/codex/oauth.js";
import { codexStreamFn } from "@/providers/src/codex/stream-fn.js";

console.log("Running Codex provider tests...");

function sse(events: Array<Record<string, unknown>>): string {
  return events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join("") + "data: [DONE]\n\n";
}

async function collectDeltas(body: string) {
  const originalFetch = globalThis.fetch;
  const originalToken = process.env.OPENAI_CODEX_OAUTH_TOKEN;
  const payload = Buffer.from(
    JSON.stringify({
      "https://api.openai.com/auth": { chatgpt_account_id: "test-account" },
    }),
  ).toString("base64url");
  process.env.OPENAI_CODEX_OAUTH_TOKEN = `header.${payload}.signature`;
  globalThis.fetch = (async () =>
    new Response(body, { headers: { "Content-Type": "text/event-stream" } })) as unknown as typeof fetch;

  try {
    const model: Model = { id: "gpt-5.6-luna", provider: "codex", contextWindow: 272_000 };
    const deltas = [];
    for await (const delta of codexStreamFn({
      model,
      systemPrompt: "Use tools when requested.",
      messages: [{ role: "user", content: "List the project directory." }],
      tools: [],
    })) {
      deltas.push(delta);
    }
    return deltas;
  } finally {
    globalThis.fetch = originalFetch;
    if (originalToken === undefined) delete process.env.OPENAI_CODEX_OAUTH_TOKEN;
    else process.env.OPENAI_CODEX_OAUTH_TOKEN = originalToken;
  }
}

// Argument deltas use item_id, while the agent's internal tool-call ID must be
// the function call's call_id. The finalized event must replace, not duplicate,
// the streamed fragments.
{
  const deltas = await collectDeltas(
    sse([
      {
        type: "response.output_item.added",
        item: { type: "function_call", id: "item-1", call_id: "call-1", name: "listDir", arguments: "" },
      },
      {
        type: "response.function_call_arguments.delta",
        item_id: "item-1",
        delta: '{"path":',
      },
      {
        type: "response.function_call_arguments.delta",
        item_id: "item-1",
        delta: '"."}',
      },
      {
        type: "response.function_call_arguments.done",
        item_id: "item-1",
        name: "listDir",
        arguments: '{"path":"."}',
      },
    ]),
  );

  const toolDeltas = deltas.filter((delta) => delta.type === "toolCall");
  assert.deepEqual(
    toolDeltas.map((delta) => ({ id: delta.id, name: delta.name, argumentsJson: delta.argumentsJson })),
    [
      { id: "call-1", name: "listDir", argumentsJson: "" },
      { id: "call-1", name: "listDir", argumentsJson: '{"path":"."}' },
    ],
  );
  assert.deepEqual(JSON.parse(toolDeltas[1]!.argumentsJson), { path: "." });
  console.log("  ✅ Codex item_id argument correlation and finalized JSON");
}

// A finalized event can arrive without any delta fragments.
{
  const deltas = await collectDeltas(
    sse([
      {
        type: "response.output_item.added",
        item: { type: "function_call", id: "item-2", call_id: "call-2", name: "listDir" },
      },
      {
        type: "response.function_call_arguments.done",
        item_id: "item-2",
        name: "listDir",
        arguments: '{"path":"."}',
      },
    ]),
  );
  const toolDeltas = deltas.filter((delta) => delta.type === "toolCall");
  assert.equal(toolDeltas[1]?.id, "call-2");
  assert.deepEqual(JSON.parse(toolDeltas[1]!.argumentsJson), { path: "." });
  console.log("  ✅ Codex finalized-only arguments");
}

// Deltas arriving before output_item.added are retained and attached to the
// correct call once its call_id becomes available.
{
  const deltas = await collectDeltas(
    sse([
      {
        type: "response.function_call_arguments.delta",
        item_id: "item-3",
        delta: '{"path":"."}',
      },
      {
        type: "response.output_item.added",
        item: { type: "function_call", id: "item-3", call_id: "call-3", name: "listDir" },
      },
    ]),
  );
  const toolDeltas = deltas.filter((delta) => delta.type === "toolCall");
  assert.equal(toolDeltas[1]?.id, "call-3");
  assert.deepEqual(JSON.parse(toolDeltas[1]!.argumentsJson), { path: "." });
  console.log("  ✅ Codex out-of-order argument buffering");
}

// Opt-in live round trip through the actual Agent loop and logged-in Codex
// subscription. This intentionally makes one real request and does not retry.
if (process.env.CODEX_REAL_API === "1") {
  if (!(await codexCredentialExists())) {
    console.warn("  ⏭️  live Codex test skipped: no Codex credentials found");
  } else {
    const model: Model = {
      id: process.env.CODEX_TEST_MODEL ?? "gpt-5.6-luna",
      provider: "codex",
      contextWindow: 272_000,
      supportsImages: true,
    };
    const boundListDir = bindToolCwd(listDirTool, process.cwd()) as unknown as AgentTool;
    const calls: Array<{ name: string; arguments: unknown }> = [];
    const events: string[] = [];
    const agent = new Agent({
      model,
      tools: [boundListDir],
      streamFn: codexStreamFn,
      systemPrompt:
        'You are a tool-calling test agent. You MUST call the provided listDir function before replying. Call it exactly once with path "." and recursive false. Do not answer from memory.',
      approvalMode: "full-access",
      onEvent: (event) => {
        events.push(event.type);
        if (event.type === "toolExecutionStart") {
          calls.push(
            ...event.calls.map((call) => ({ name: call.name, arguments: call.arguments })),
          );
        }
      },
    });

    const stream = agent.run(
      'Use the listDir tool now with path "." and recursive false. Then reply "done".',
      AbortSignal.timeout(180_000),
    );
    for await (const _event of stream) {
      // Consume the event stream until the real agent loop completes.
    }
    await stream.result();

    assert.equal(
      calls.length,
      1,
      `expected one Codex tool call, got ${calls.length}; events=${events.join(",")}`,
    );
    assert.equal(calls[0]?.name, "listDir");
    assert.deepEqual(calls[0]?.arguments, { path: ".", recursive: false });
    console.log("  ✅ live Codex gpt-5.6-luna listDir tool call");
  }
}

console.log("Codex provider tests passed!\n");
