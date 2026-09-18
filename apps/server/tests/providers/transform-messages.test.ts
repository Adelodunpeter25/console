import assert from "node:assert/strict";
import type { AgentMessage } from "@console/types";
import {
  transformMessages,
  INTERRUPTED_TOOL_RESULT_TEXT,
  DEFAULT_CONTINUE_PROMPT,
  DEFAULT_SESSION_START_PROMPT,
} from "@/providers/src/shared/transform-messages.js";
import { convertClaudeMessages } from "@/providers/src/claude/convert.js";

console.log("Running transform-messages tests...");

// 1. Interrupted tool calls synthesize toolResult immediately following assistant turn
{
  const input: AgentMessage[] = [
    { role: "user", content: "read a file" },
    {
      role: "assistant",
      content: [
        {
          type: "toolCall",
          call: { id: "call_123", name: "readFile", arguments: { path: "test.txt" } },
        },
      ],
      stopReason: "toolUse",
    },
    { role: "user", content: "actually never mind" },
  ];

  const result = transformMessages(input);
  assert.equal(result.length, 4);
  assert.equal(result[0]!.role, "user");
  assert.equal(result[1]!.role, "assistant");
  assert.equal(result[2]!.role, "toolResult");
  if (result[2]!.role === "toolResult") {
    assert.equal(result[2]!.results.length, 1);
    assert.equal(result[2]!.results[0]!.toolCallId, "call_123");
    assert.equal(result[2]!.results[0]!.isError, true);
    assert.equal(result[2]!.results[0]!.content, INTERRUPTED_TOOL_RESULT_TEXT);
  }
  assert.equal(result[3]!.role, "user");
  console.log("  ✅ Synthesizes toolResult for interrupted tool call before next user turn");
}

// 2. Trailing assistant turn with tool call gets synthetic toolResult
{
  const input: AgentMessage[] = [
    { role: "user", content: "run command" },
    {
      role: "assistant",
      content: [
        {
          type: "toolCall",
          call: { id: "call_cmd", name: "bash", arguments: { command: "ls" } },
        },
      ],
      stopReason: "toolUse",
    },
  ];

  const result = transformMessages(input);
  assert.equal(result.length, 3);
  assert.equal(result[0]!.role, "user");
  assert.equal(result[1]!.role, "assistant");
  assert.equal(result[2]!.role, "toolResult");
  if (result[2]!.role === "toolResult") {
    assert.equal(result[2]!.results[0]!.toolCallId, "call_cmd");
    assert.equal(result[2]!.results[0]!.isError, true);
  }

  // And verify Claude wire conversion maps this to ending with a user turn
  const wire = convertClaudeMessages(input);
  assert.equal(wire.length, 3);
  assert.equal(wire[2]!.role, "user");
  console.log("  ✅ Trailing assistant toolCall gets synthetic toolResult and user wire termination");
}

// 3. Trailing assistant text turn gets user continuation
{
  const input: AgentMessage[] = [
    { role: "user", content: "hello" },
    {
      role: "assistant",
      content: [{ type: "text", text: "Hello! How can I help?" }],
      stopReason: "stop",
    },
  ];

  const result = transformMessages(input);
  assert.equal(result.length, 3);
  assert.equal(result[0]!.role, "user");
  assert.equal(result[1]!.role, "assistant");
  assert.equal(result[2]!.role, "user");
  if (result[2]!.role === "user") {
    assert.equal(result[2]!.content, DEFAULT_CONTINUE_PROMPT);
  }
  console.log("  ✅ Trailing assistant turn gets user continuation prompt");
}

// 4. Missing tool results are augmented when toolResult is only partially answered
{
  const input: AgentMessage[] = [
    { role: "user", content: "do two things" },
    {
      role: "assistant",
      content: [
        {
          type: "toolCall",
          call: { id: "call_a", name: "toolA", arguments: {} },
        },
        {
          type: "toolCall",
          call: { id: "call_b", name: "toolB", arguments: {} },
        },
      ],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [{ toolCallId: "call_a", content: "result a" }],
    },
  ];

  const result = transformMessages(input);
  // toolResult is augmented with call_b synthetic error, and trailing toolResult is followed by user continue
  assert.equal(result[1]!.role, "assistant");
  assert.equal(result[2]!.role, "toolResult");
  if (result[2]!.role === "toolResult") {
    assert.equal(result[2]!.results.length, 2);
    assert.equal(result[2]!.results[0]!.toolCallId, "call_a");
    assert.equal(result[2]!.results[1]!.toolCallId, "call_b");
    assert.equal(result[2]!.results[1]!.isError, true);
  }
  console.log("  ✅ Partially answered toolResult augmented with missing tool results");
}

// 5. Orphaned toolResult messages convert into readable user text
{
  const input: AgentMessage[] = [
    {
      role: "toolResult",
      results: [{ toolCallId: "orphan_call", toolName: "git", content: "commit output" }],
    },
    { role: "user", content: "next prompt" },
  ];

  const result = transformMessages(input);
  assert.equal(result.length, 1); // merged into single user message
  assert.equal(result[0]!.role, "user");
  if (result[0]!.role === "user") {
    assert.ok(result[0]!.content.includes("[Tool result for git: commit output]"));
    assert.ok(result[0]!.content.includes("next prompt"));
  }
  console.log("  ✅ Orphaned toolResult converted to readable user text block");
}

// 6. Leading assistant turn prepends user starter
{
  const input: AgentMessage[] = [
    {
      role: "assistant",
      content: [{ type: "text", text: "Restored from snapshot" }],
      stopReason: "stop",
    },
  ];

  const result = transformMessages(input);
  assert.equal(result.length, 3);
  assert.equal(result[0]!.role, "user");
  if (result[0]!.role === "user") {
    assert.equal(result[0]!.content, DEFAULT_SESSION_START_PROMPT);
  }
  assert.equal(result[1]!.role, "assistant");
  assert.equal(result[2]!.role, "user");
  console.log("  ✅ Leading assistant turn prepends user session starter");
}

// 7. Empty message array returns single user turn
{
  const result = transformMessages([]);
  assert.equal(result.length, 1);
  assert.equal(result[0]!.role, "user");
  if (result[0]!.role === "user") {
    assert.equal(result[0]!.content, DEFAULT_CONTINUE_PROMPT);
  }
  console.log("  ✅ Empty input returns single user turn");
}

// 8. Truncated thinking-only assistant turn is pruned
{
  const input: AgentMessage[] = [
    { role: "user", content: "solve equation" },
    {
      role: "assistant",
      content: [{ type: "thinking", text: "let me think" }],
      stopReason: "aborted",
    },
    { role: "user", content: "try this instead" },
  ];

  const result = transformMessages(input);
  assert.equal(result.length, 1); // two user turns merged into 1
  assert.equal(result[0]!.role, "user");
  console.log("  ✅ Truncated thinking-only assistant turn pruned");
}

// 9. Demoted thinking trailing whitespace is trimmed
{
  const input: AgentMessage[] = [
    { role: "user", content: "think" },
    {
      role: "assistant",
      content: [{ type: "thinking", text: "step 1 reasoning   \n\t  " }],
      stopReason: "stop",
    },
  ];

  const result = transformMessages(input);
  assert.equal(result[1]!.role, "assistant");
  if (result[1]!.role === "assistant") {
    assert.equal(result[1]!.content[0]!.type, "text");
    assert.equal((result[1]!.content[0] as any).text, "step 1 reasoning");
  }
  console.log("  ✅ Demoted thinking trailing whitespace is trimmed");
}

// 10. Claude wire format guarantee: convertClaudeMessages always ends with role: "user"
{
  // Test case with dangling assistant tool call at the end
  const wire = convertClaudeMessages([
    { role: "user", content: "inspect project" },
    {
      role: "assistant",
      content: [
        {
          type: "toolCall",
          call: { id: "tc_dangling", name: "readFile", arguments: { path: "main.ts" } },
        },
      ],
      stopReason: "toolUse",
    },
  ]);

  assert.ok(wire.length >= 3);
  assert.equal(wire[0]!.role, "user");
  assert.equal(wire[1]!.role, "assistant");
  assert.equal(wire[wire.length - 1]!.role, "user", "Claude wire MUST end with role: user");

  // Verify that the tool_use in wire[1] is immediately followed by tool_result in wire[2]
  const assistantBlock = wire[1]!.content[0] as any;
  assert.equal(assistantBlock.type, "tool_use");
  const toolResultBlock = wire[2]!.content[0] as any;
  assert.equal(toolResultBlock.type, "tool_result");
  assert.equal(toolResultBlock.tool_use_id, "tc_dangling");
  assert.equal(toolResultBlock.is_error, true);
  console.log("  ✅ convertClaudeMessages guarantees compliant tool_use/tool_result and user termination");
}

console.log("All transform-messages tests passed!\n");
