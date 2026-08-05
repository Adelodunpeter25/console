import type { AgentMessage, ToolCall, ToolResult, ToolResultMessage } from "../types/index.js";

const INTERRUPTED_RESULT = "Tool execution was interrupted before a result was recorded.";

function toolCalls(message: AgentMessage): ToolCall[] {
  if (message.role !== "assistant") return [];
  return message.content
    .filter((part): part is Extract<typeof part, { type: "toolCall" }> => part.type === "toolCall")
    .map((part) => part.call);
}

function interruptedResult(call: ToolCall): ToolResult {
  return {
    toolCallId: call.id,
    toolName: call.name,
    content: INTERRUPTED_RESULT,
    isError: true,
  };
}

/**
 * Repairs histories interrupted between an assistant tool call and its result.
 * Providers such as Anthropic require the result message to be immediately
 * after the assistant tool-use message, so this repair is also applied before
 * retrying a session after a crash or cancelled approval.
 */
export function repairToolCallHistory(messages: AgentMessage[]): {
  messages: AgentMessage[];
  repaired: boolean;
} {
  const repairedMessages: AgentMessage[] = [];
  let repaired = false;

  for (let index = 0; index < messages.length; index++) {
    const message = messages[index]!;
    repairedMessages.push(message);

    const calls = toolCalls(message);
    if (calls.length === 0) continue;

    const next = messages[index + 1];
    if (next?.role !== "toolResult") {
      repairedMessages.push({
        role: "toolResult",
        results: calls.map(interruptedResult),
      });
      repaired = true;
      continue;
    }

    const existingIds = new Set(next.results.map((result) => result.toolCallId));
    const missing = calls.filter((call) => !existingIds.has(call.id));
    if (missing.length > 0) {
      const repairedResult: ToolResultMessage = {
        role: "toolResult",
        results: [...next.results, ...missing.map(interruptedResult)],
      };
      repairedMessages.push(repairedResult);
      repaired = true;
      index++;
    }
  }

  return { messages: repaired ? repairedMessages : messages, repaired };
}
