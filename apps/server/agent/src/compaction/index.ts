/**
 * Context Window Compaction Engine.
 * Monitors token usage and condenses old turns when approaching context limits.
 * Inspired by oh-my-pi/packages/agent/src/compaction/ & oh-my-pi/packages/coding-agent/src/compaction/.
 */
import type { AgentMessage, Model } from "@/agent/src/types/index.js";

export interface CompactionOptions {
  /** Maximum ratio of contextWindow before auto-compaction triggers. Default: 0.8 (80%) */
  maxThresholdRatio?: number;
  /** Keep the most recent N message turns uncompacted. Default: 4 */
  keepRecentTurns?: number;
  /** Hard token threshold override. */
  tokenThreshold?: number;
}

/**
 * Estimate token count for a list of messages (~4 chars per token).
 */
export function estimateMessageTokens(messages: AgentMessage[]): number {
  let totalChars = 0;

  for (const msg of messages) {
    if (msg.role === "user") {
      totalChars += msg.content.length;
    } else if (msg.role === "assistant") {
      for (const part of msg.content) {
        if (part.type === "text" || part.type === "thinking") {
          totalChars += part.text.length;
        } else if (part.type === "toolCall") {
          totalChars += part.call.name.length + JSON.stringify(part.call.arguments).length;
        }
      }
    } else if (msg.role === "toolResult") {
      for (const res of msg.results) {
        totalChars += JSON.stringify(res.content).length;
      }
    }
  }

  return Math.ceil(totalChars / 4);
}

/**
 * Determine if conversation history requires compaction.
 */
export function shouldCompact(
  messages: AgentMessage[],
  model: Model,
  options: CompactionOptions = {},
): boolean {
  const { maxThresholdRatio = 0.8, tokenThreshold } = options;
  const tokens = estimateMessageTokens(messages);
  const limit = tokenThreshold ?? Math.floor(model.contextWindow * maxThresholdRatio);

  return tokens >= limit;
}

/**
 * Compact conversation history by summarizing older turns into a single checkpoint turn.
 */
export function compactHistory(
  messages: AgentMessage[],
  options: CompactionOptions = {},
): { compactedMessages: AgentMessage[]; summary: string; originalCount: number } {
  const keepRecent = options.keepRecentTurns ?? 4;
  if (messages.length <= keepRecent + 2) {
    return {
      compactedMessages: [...messages],
      summary: "History too short to compact.",
      originalCount: messages.length,
    };
  }

  const splitIndex = Math.max(0, messages.length - keepRecent);
  const olderMessages = messages.slice(0, splitIndex);
  const recentMessages = messages.slice(splitIndex);

  const userCount = olderMessages.filter((m) => m.role === "user").length;
  const assistantCount = olderMessages.filter((m) => m.role === "assistant").length;
  const toolResultCount = olderMessages.filter((m) => m.role === "toolResult").length;

  const summary = `[Conversation Checkpoint: Compacted ${olderMessages.length} prior messages (${userCount} user prompts, ${assistantCount} assistant turns, ${toolResultCount} tool results).]`;

  const summaryUserMessage: AgentMessage = {
    role: "user",
    content: "Summarize conversation checkpoint",
  };

  const summaryAssistantMessage: AgentMessage = {
    role: "assistant",
    id: crypto.randomUUID(),
    content: [{ type: "text", text: summary }],
    stopReason: "stop",
  };

  const compactedMessages: AgentMessage[] = [
    summaryUserMessage,
    summaryAssistantMessage,
    ...recentMessages,
  ];

  return {
    compactedMessages,
    summary,
    originalCount: messages.length,
  };
}
