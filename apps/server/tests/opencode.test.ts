/**
 * Unit Tests for the OpenCode Zen (opencode) provider.
 * Operates offline — uses mock fetch responses (0 LLM credits consumed).
 *
 * Covers:
 *  1. convertOpencodeMessages → OpenAI wire format (system/user/assistant/tool)
 *  2. convertOpencodeTools → OpenAI function tools (JSON Schema parameters)
 *  3. opencodeStreamFn → SSE chunk parsing into text / thinking / toolCall deltas
 *  4. fetchOpencodeFreeModels → free-tier filtering from /v1/models payload
 *  5. provider-registry catalog entry for "opencode"
 */
import assert from "node:assert/strict";
import type { AgentMessage, AgentTool } from "../agent/src/types/index.js";
import { z } from "zod";
import { listProviders, listModelsForProvider } from "../agent/src/commands/provider-registry.js";
import {
  convertOpencodeMessages,
  type OpenAIInputMessage,
} from "../providers/src/opencode/convert-messages.js";
import { convertOpencodeTools } from "../providers/src/opencode/convert-tools.js";
import { opencodeStreamFn } from "../providers/src/opencode/stream-fn.js";
import { fetchOpencodeFreeModels } from "../providers/src/opencode/discovery.js";

console.log("Running OpenCode Zen (opencode) Provider tests...");

// 1. Message converter (convertOpencodeMessages)
{
  const messages: AgentMessage[] = [
    { role: "user", content: "List files" },
    {
      role: "assistant",
      id: "turn-1",
      content: [
        { type: "thinking", text: "I should list the files." },
        { type: "text", text: "Listing now." },
        {
          type: "toolCall",
          call: { id: "call_123", name: "listDir", arguments: { path: "." } },
        },
      ],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [{ toolCallId: "call_123", content: { files: ["a.ts", "b.ts"] }, isError: false }],
    },
  ];

  const wire = convertOpencodeMessages(messages, "You are a terse assistant.");

  assert.equal(wire.length, 4);
  assert.equal(wire[0]?.role, "system");
  assert.equal(wire[0]?.content, "You are a terse assistant.");
  assert.equal(wire[1]?.role, "user");
  assert.equal(wire[1]?.content, "List files");

  const assistant = wire[2] as OpenAIInputMessage;
  assert.equal(assistant.role, "assistant");
  assert.equal(assistant.reasoning_content, "I should list the files.");
  assert.equal(assistant.content, "Listing now.");
  assert.equal(assistant.tool_calls?.[0]?.function.name, "listDir");
  assert.equal(assistant.tool_calls?.[0]?.id, "call_123");
  assert.equal(assistant.tool_calls?.[0]?.function.arguments, '{"path":"."}');

  const toolMsg = wire[3] as OpenAIInputMessage;
  assert.equal(toolMsg.role, "tool");
  assert.equal(toolMsg.tool_call_id, "call_123");
  assert.equal(toolMsg.content, '{"files":["a.ts","b.ts"]}');
  console.log("  ✅ convertOpencodeMessages wire transformation");
}

// 2. Tool converter (convertOpencodeTools)
{
  const sampleTool: AgentTool = {
    name: "searchCode",
    description: "Search codebase using regex pattern",
    inputSchema: z.object({
      pattern: z.string().describe("RegEx pattern to search"),
      maxResults: z.number().optional().default(10),
    }),
    execute: async () => {},
  };

  const wireTools = convertOpencodeTools([sampleTool]);
  assert.equal(wireTools.length, 1);
  assert.equal(wireTools[0]?.type, "function");
  assert.equal(wireTools[0]?.function.name, "searchCode");
  assert.equal(wireTools[0]?.function.description, "Search codebase using regex pattern");
  const toolParams = wireTools[0]?.function.parameters as Record<string, unknown>;
  assert.ok(toolParams?.properties);
  assert.ok((toolParams.properties as Record<string, unknown>).pattern);
  console.log("  ✅ convertOpencodeTools Zod to JSON Schema conversion");
}

// 3. opencodeStreamFn — SSE chunk parsing into deltas (mock fetch)
{
  const originalFetch = globalThis.fetch;
  const mockSse = [
    'data: {"choices":[{"delta":{"reasoning_content":"Let me think"}}]}',
    "",
    'data: {"choices":[{"delta":{"content":"pineapple"}}]}',
    "",
    'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"listDir","arguments":"{\\"path\\":"}}]}}]}',
    "",
    'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\".\\"}"}}]}}]}',
    "",
    "data: [DONE]",
  ].join("\n");

  globalThis.fetch = (async () =>
    new Response(mockSse, {
      headers: { "Content-Type": "text/event-stream" },
    })) as typeof fetch;

  try {
    const deltas: Array<{ type: string; text?: string; name?: string; argumentsJson?: string }> = [];
    for await (const delta of opencodeStreamFn({
      model: { id: "deepseek-v4-flash-free", provider: "opencode", contextWindow: 128_000 },
      systemPrompt: "Be terse.",
      messages: [{ role: "user", content: "hi" }],
      tools: [],
    })) {
      deltas.push(delta);
    }

    assert.equal(deltas.length, 4);
    assert.equal(deltas[0]?.type, "thinking");
    assert.equal((deltas[0] as { text: string }).text, "Let me think");
    assert.equal(deltas[1]?.type, "text");
    assert.equal((deltas[1] as { text: string }).text, "pineapple");
    assert.equal(deltas[2]?.type, "toolCall");
    assert.equal((deltas[2] as { name: string }).name, "listDir");
    assert.equal((deltas[2] as { argumentsJson: string }).argumentsJson, '{"path":');
    // Second tool-call fragment accumulates onto the same id
    assert.equal(deltas[3]?.type, "toolCall");
    assert.equal((deltas[3] as { id: string }).id, "call_1");
    assert.equal((deltas[3] as { argumentsJson: string }).argumentsJson, '"."}');
    console.log("  ✅ opencodeStreamFn SSE streaming (thinking/text/toolCall)");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 4. fetchOpencodeFreeModels — free-tier filtering from /v1/models payload
{
  const originalFetch = globalThis.fetch;
  const mockPayload = {
    object: "list",
    data: [
      { id: "deepseek-v4-flash-free", object: "model" },
      { id: "big-pickle", object: "model" },
      { id: "claude-opus-5", object: "model" },
      { id: "gpt-5.4", object: "model" },
    ],
  };

  globalThis.fetch = (async () =>
    new Response(JSON.stringify(mockPayload), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })) as typeof fetch;

  try {
    const models = await fetchOpencodeFreeModels();
    const ids = models.map((m) => m.id).sort();
    assert.deepEqual(ids, ["big-pickle", "deepseek-v4-flash-free"]);
    assert.ok(models.every((m) => m.provider === "opencode"));
    console.log("  ✅ fetchOpencodeFreeModels free-tier filtering");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 5. Provider-registry catalog entry for "opencode"
{
  const providers = listProviders();
  const zenEntry = providers.find((p) => p.name === "opencode");
  assert.ok(zenEntry, "opencode should be registered in the provider catalog");
  assert.equal(zenEntry!.displayName, "OpenCode Zen");

  const models = listModelsForProvider("opencode");
  assert.ok(models.length > 0);
  assert.ok(models.some((m) => m.id === "deepseek-v4-flash-free"));
  assert.ok(models.every((m) => m.provider === "opencode"));
  console.log("  ✅ provider-registry opencode catalog entry");
}

console.log("OpenCode Zen (opencode) Provider tests passed!\n");
