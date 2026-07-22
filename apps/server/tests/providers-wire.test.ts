/**
 * Unit Tests for Provider Wire Converters & SSE Parser.
 * Zero network calls — tests JSON serialization and wire mappings locally.
 */
import assert from "node:assert/strict";
import type { AgentMessage, AgentTool } from "../agent/src/types/index.js";
import { convertMessages, convertTools, parseSse } from "../providers/src/shared/index.js";
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

  const wireContent = convertMessages(messages);
  assert.equal(wireContent.length, 3);
  assert.equal(wireContent[0]?.role, "user");
  assert.equal(wireContent[1]?.role, "model");
  assert.equal(wireContent[2]?.role, "user");

  // Verify function call wire structure
  assert.ok("functionCall" in wireContent[1]!.parts[1]!);
  assert.equal((wireContent[1]!.parts[1] as any).functionCall.name, "listDir");

  // Verify function response wire structure
  assert.ok("functionResponse" in wireContent[2]!.parts[0]!);
  assert.equal((wireContent[2]!.parts[0] as any).functionResponse.id, "call_123");
  console.log("  ✅ convertMessages wire transformation");
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

console.log("Provider Wire Converter tests passed!\n");
