import type { AgentMessage } from "@/agent/src/types/index.js";

/**
 * Mechanically reduce old tool output before throwing away whole conversation turns.
 * The protected suffix is intentionally left byte-for-byte unchanged.
 */
export function shakeConversation(
  messages: AgentMessage[],
  maxToolResultChars = 8_000,
  protectedFromIndex = messages.length,
): AgentMessage[] {
  let changed = false;
  const shaken = messages.map((message, index) => {
    if (index >= protectedFromIndex || message.role !== "toolResult") return message;

    const results = message.results.map((result) => {
      if (result.isError || typeof result.content !== "string" || result.content.length <= maxToolResultChars) {
        return result;
      }
      changed = true;
      return {
        ...result,
        content: `${result.content.slice(0, maxToolResultChars)}\n[…tool output shaken for context space…]`,
      };
    });

    return results.some((result, resultIndex) => result !== message.results[resultIndex])
      ? { ...message, results }
      : message;
  });

  return changed ? shaken : messages;
}

/** Earliest message in the final `turnCount` user turns. */
export function protectedRecentStart(messages: AgentMessage[], turnCount = 3): number {
  const userIndices: number[] = [];
  for (let index = 0; index < messages.length; index++) {
    if (messages[index].role === "user") userIndices.push(index);
  }
  return userIndices.length > turnCount ? userIndices[userIndices.length - turnCount] : 0;
}
