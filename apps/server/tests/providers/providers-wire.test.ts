/**
 * Unit Tests for Provider Wire Converters & SSE Parser.
 * Zero network calls — tests JSON serialization and wire mappings locally.
 */
import assert from "node:assert/strict";
import type { AgentMessage, AgentTool } from "@console/types";
import {
  convertMessages,
  convertTools,
  LEGACY_THOUGHT_SIGNATURE,
  parseSse,
  streamCore,
} from "@/providers/src/shared/index.js";
import { z } from "zod";

console.log("Running Provider Wire Converter tests...");

// 1. Message converter (convertMessages)
{
  const messages: AgentMessage[] = [
    { role: "user", content: "List files" },
    {
      role: "assistant",
      id: "turn-1",
      content: [
        { type: "text", text: "I will list files." },
        {
          type: "toolCall",
          call: {
            id: "call_123",
            name: "listDir",
            arguments: { path: "." },
            thoughtSignature: "signature-A",
          },
        },
      ],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [{ toolCallId: "call_123", content: { files: ["a.ts", "b.ts"] }, isError: false }],
    },
  ];

  const wireContent = convertMessages(messages);
  assert.equal(wireContent.length, 3);
  assert.equal(wireContent[0]?.role, "user");
  assert.equal(wireContent[1]?.role, "model");
  assert.equal(wireContent[2]?.role, "user");

  // Verify function call wire structure
  assert.ok("functionCall" in wireContent[1]!.parts[1]!);
  assert.equal((wireContent[1]!.parts[1] as any).functionCall.name, "listDir");
  assert.equal((wireContent[1]!.parts[1] as any).thoughtSignature, "signature-A");

  // Verify function response wire structure
  assert.ok("functionResponse" in wireContent[2]!.parts[0]!);
  assert.equal((wireContent[2]!.parts[0] as any).functionResponse.name, "listDir");
  assert.equal((wireContent[2]!.parts[0] as any).functionResponse.id, "call_123");
  console.log("  ✅ convertMessages wire transformation");
}

// 1a. Multi-turn same-role merging and trailing model turn stripping (prefill prevention)
{
  const multiTurnMessages: AgentMessage[] = [
    { role: "user", content: "Prompt 1" },
    {
      role: "assistant",
      id: "turn-1",
      content: [
        {
          type: "toolCall",
          call: { id: "call-a", name: "bash", arguments: { cmd: "ls" } },
        },
      ],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [{ toolCallId: "call-a", content: "file.txt" }],
    },
    { role: "user", content: "checkout the codebase" },
  ];

  const wire = convertMessages(multiTurnMessages);
  // ToolResult (user) + UserMessage (user) should be merged into a single user turn:
  // [user (Prompt 1), model (toolCall), user (functionResponse + "checkout the codebase")]
  assert.equal(wire.length, 3);
  assert.equal(wire[0]?.role, "user");
  assert.equal(wire[1]?.role, "model");
  assert.equal(wire[2]?.role, "user");
  assert.equal(wire[2]?.parts.length, 2);
  assert.ok("functionResponse" in wire[2]!.parts[0]!);
  assert.equal((wire[2]!.parts[0] as any).functionResponse.name, "bash");
  assert.equal((wire[2]!.parts[1] as any).text, "checkout the codebase");
  console.log("  ✅ Multi-turn consecutive user turn merging");

  // Trailing model message (e.g. from a finished subagent) must be dropped so request ends with user
  const prefillRiskMessages: AgentMessage[] = [
    ...multiTurnMessages,
    {
      role: "assistant",
      id: "turn-2",
      content: [{ type: "text", text: "Finished subagents." }],
      stopReason: "stop",
    },
  ];
  const strippedWire = convertMessages(prefillRiskMessages);
  assert.equal(strippedWire[strippedWire.length - 1]?.role, "user");
  console.log("  ✅ Trailing model turn dropped (assistant prefill prevention)");

  const assistantOnlyWire = convertMessages(
    [
      {
        role: "assistant",
        id: "turn-only",
        content: [{ type: "text", text: "stale response" }],
        stopReason: "stop",
      },
    ],
    { requireUserTerminator: true },
  );
  // Assistant-only history prepends/appends user boundaries without dropping the model context.
  assert.equal(assistantOnlyWire.length, 3);
  assert.equal(assistantOnlyWire[0]?.role, "user");
  assert.equal(assistantOnlyWire[1]?.role, "model");
  assert.equal(assistantOnlyWire[2]?.role, "user");
  console.log("  ✅ Assistant-only history preserves model turn with user boundaries");

  // An image-only user message (empty text + attachments) must survive
  // conversion — dropping it can strand a trailing assistant turn.
  const imageOnlyWire = convertMessages(
    [
      {
        role: "assistant",
        id: "turn-1",
        content: [{ type: "text", text: "Here is what I found." }],
        stopReason: "stop",
      },
      {
        role: "user",
        content: "",
        attachments: [{ type: "image", data: "aGVsbG8=", mimeType: "image/png" }],
      },
    ],
    { requireUserTerminator: true },
  );
  assert.equal(imageOnlyWire[imageOnlyWire.length - 1]?.role, "user");
  assert.equal(imageOnlyWire[imageOnlyWire.length - 1]?.parts.length, 2);
  console.log("  ✅ Image-only user message preserved (prefill prevention)");
}

// 1b. Legacy function calls receive the documented compatibility sentinel.
{
  const wireContent = convertMessages([
    {
      role: "assistant",
      id: "turn-legacy",
      content: [
        {
          type: "toolCall",
          call: { id: "legacy-call", name: "todo", arguments: {} },
        },
      ],
      stopReason: "toolUse",
    },
  ]);

  const modelTurn = wireContent.find((t) => t.role === "model")!;
  assert.equal((modelTurn.parts[0] as any).thoughtSignature, LEGACY_THOUGHT_SIGNATURE);
  console.log("  ✅ Legacy thought-signature fallback");
}

// 1c. Orphaned tool results (e.g. at conversation start or severed from assistant turn)
// are converted to user text parts so CCA/Claude models do not reject unexpected tool_use_ids.
{
  // History starts with assistant tool call + tool result. Leading user starter is prepended,
  // preserving the tool call and result.
  const leadingToolUseMessages: AgentMessage[] = [
    {
      role: "assistant",
      id: "turn-leading",
      content: [
        {
          type: "toolCall",
          call: { id: "toolu_vrtx_123", name: "readFile", arguments: { path: "a.ts" } },
        },
      ],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [{ toolCallId: "toolu_vrtx_123", toolName: "readFile", content: "file content" }],
    },
    {
      role: "user",
      content: "continue working",
    },
  ];

  const wire = convertMessages(leadingToolUseMessages, { requireUserTerminator: true });
  assert.equal(wire.length, 3);
  assert.equal(wire[0]?.role, "user");
  assert.equal(wire[1]?.role, "model");
  assert.equal(wire[2]?.role, "user");
  console.log("  ✅ Leading toolCall and toolResult preserved with user starter");

  // Orphaned tool result alone
  const orphanedAlone = convertMessages([
    {
      role: "toolResult",
      results: [{ toolCallId: "orphan_call_999", toolName: "bash", content: "cmd output" }],
    },
  ]);
  assert.equal(orphanedAlone.length, 1);
  assert.equal(orphanedAlone[0]?.role, "user");
  assert.equal("functionResponse" in (orphanedAlone[0]?.parts[0] ?? {}), false);
  console.log("  ✅ Standalone toolResult converted to safe user text part");
}

// 2. Tool converter (convertTools)
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

  const wireTools = convertTools([sampleTool]);
  assert.equal(wireTools.length, 1);
  assert.equal(wireTools[0]?.name, "searchCode");
  assert.equal(wireTools[0]?.description, "Search codebase using regex pattern");
  const toolParams = wireTools[0]?.parameters as any;
  assert.ok(toolParams?.properties);
  assert.ok(toolParams.properties.pattern);
  console.log("  ✅ convertTools Zod to JSON Schema conversion");
}

// 3. SSE Parser (parseSse)
{
  const mockSseText = [
    'data: {"response": {"candidates": [{"content": {"parts": [{"text": "Hello"}]}}]}}',
    "",
    'data: {"response": {"candidates": [{"content": {"parts": [{"text": " World"}]}}]}}',
    "data: [DONE]",
  ].join("\n");

  const response = new Response(mockSseText, {
    headers: { "Content-Type": "text/event-stream" },
  });

  const parsedChunks: any[] = [];
  for await (const chunk of parseSse(response)) {
    parsedChunks.push(chunk);
  }

  assert.equal(parsedChunks.length, 2);
  assert.equal(parsedChunks[0].response.candidates[0].content.parts[0].text, "Hello");
  assert.equal(parsedChunks[1].response.candidates[0].content.parts[0].text, " World");
  console.log("  ✅ parseSse text/event-stream chunk parser");
}

// 4. CCA stream preserves signatures on model parts.
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      'data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"todo","args":{},"id":"call-1"},"thoughtSignature":"signature-stream"}]}}]}}\n\n',
      { headers: { "Content-Type": "text/event-stream" } },
    )) as unknown as typeof fetch;

  try {
    const deltas = [];
    for await (const delta of streamCore({
      endpoint: "https://example.test",
      accessToken: "token",
      extraHeaders: {},
      body: {} as any,
      signal: undefined,
    })) {
      deltas.push(delta);
    }
    assert.equal(deltas[0]?.type, "toolCall");
    assert.equal((deltas[0] as any).thoughtSignature, "signature-stream");
    console.log("  ✅ CCA thought-signature streaming");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 5. CCA usage normalization: cache hit → cacheStatus "hit", cache read
// subtracted from uncached input. Reasoning tokens preserved when reported.
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      'data: {"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1000,"cachedContentTokenCount":750,"candidatesTokenCount":120,"thoughtsTokenCount":40,"totalTokenCount":410}}}\n\n',
      { headers: { "Content-Type": "text/event-stream" } },
    )) as unknown as typeof fetch;

  try {
    const deltas: any[] = [];
    for await (const delta of streamCore({
      endpoint: "https://example.test",
      accessToken: "token",
      extraHeaders: {},
      body: {} as any,
      signal: undefined,
    })) {
      deltas.push(delta);
    }
    const usage = deltas.find((d) => d.type === "usage")?.usage;
    assert.ok(usage, "expected a final usage delta");
    assert.equal(usage.input, 250); // 1000 - 750
    assert.equal(usage.cacheRead, 750);
    assert.equal(usage.output, 120);
    assert.equal(usage.reasoningTokens, 40);
    assert.equal(usage.totalTokens, 410);
    assert.equal(usage.cacheStatus, "hit");
    console.log("  ✅ CCA usage: cache hit reports read + uncached + reasoning");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 6. CCA usage normalization: cachedContentTokenCount === 0 with promptTokenCount
// reported → cacheStatus "miss" (not "unknown") per the prompt-cache plan.
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      'data: {"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":500,"cachedContentTokenCount":0,"candidatesTokenCount":80,"totalTokenCount":580}}}\n\n',
      { headers: { "Content-Type": "text/event-stream" } },
    )) as unknown as typeof fetch;

  try {
    const deltas: any[] = [];
    for await (const delta of streamCore({
      endpoint: "https://example.test",
      accessToken: "token",
      extraHeaders: {},
      body: {} as any,
      signal: undefined,
    })) {
      deltas.push(delta);
    }
    const usage = deltas.find((d) => d.type === "usage")?.usage;
    assert.ok(usage, "expected a final usage delta");
    assert.equal(usage.input, 500);
    assert.equal(usage.cacheRead, 0);
    assert.equal(usage.cacheStatus, "miss");
    console.log("  ✅ CCA usage: explicit cache miss with prompt+cached fields");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 7. CCA usage normalization: missing usageMetadata entirely → cacheStatus
// "unknown" (NOT a forced miss). This is the Step 3 contract.
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      'data: {"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}}\n\n',
      { headers: { "Content-Type": "text/event-stream" } },
    )) as unknown as typeof fetch;

  try {
    const deltas: any[] = [];
    for await (const delta of streamCore({
      endpoint: "https://example.test",
      accessToken: "token",
      extraHeaders: {},
      body: {} as any,
      signal: undefined,
    })) {
      deltas.push(delta);
    }
    const usage = deltas.find((d) => d.type === "usage")?.usage;
    assert.ok(usage, "expected a final usage delta even without usageMetadata");
    assert.equal(usage.cacheStatus, "unknown");
    assert.equal(usage.cacheRead, 0);
    assert.equal(usage.input, 0);
    console.log("  ✅ CCA usage: missing metadata reports cacheStatus unknown, not miss");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 8. CCA usage normalization: prompt reported but cachedContentTokenCount
// omitted → we cannot separate, so cacheStatus is "unknown".
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      'data: {"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":300,"candidatesTokenCount":50,"totalTokenCount":350}}}\n\n',
      { headers: { "Content-Type": "text/event-stream" } },
    )) as unknown as typeof fetch;

  try {
    const deltas: any[] = [];
    for await (const delta of streamCore({
      endpoint: "https://example.test",
      accessToken: "token",
      extraHeaders: {},
      body: {} as any,
      signal: undefined,
    })) {
      deltas.push(delta);
    }
    const usage = deltas.find((d) => d.type === "usage")?.usage;
    assert.ok(usage);
    assert.equal(usage.input, 300);
    assert.equal(usage.cacheRead, 0);
    assert.equal(usage.cacheStatus, "unknown");
    console.log("  ✅ CCA usage: missing cached field keeps cacheStatus unknown");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 9. cacheRetention: "none" downgrades missing metadata to "unsupported".
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      'data: {"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}}\n\n',
      { headers: { "Content-Type": "text/event-stream" } },
    )) as unknown as typeof fetch;

  try {
    const deltas: any[] = [];
    for await (const delta of streamCore({
      endpoint: "https://example.test",
      accessToken: "token",
      extraHeaders: {},
      body: {} as any,
      signal: undefined,
      cacheRetention: "none",
    })) {
      deltas.push(delta);
    }
    const usage = deltas.find((d) => d.type === "usage")?.usage;
    assert.equal(usage.cacheStatus, "unsupported");
    console.log("  ✅ CCA usage: cacheRetention=none reports unsupported");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

console.log("Provider Wire Converter tests passed!\n");
