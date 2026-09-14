/**
 * Context Window Compaction Engine.
 * Monitors token usage, elides bloated tool results, and compacts old turns
 * into structured checkpoint summaries with cumulative file tracking.
 *
 * Inspired by oh-my-pi/packages/agent/src/compaction/.
 */
import crypto from "node:crypto";
import type { AgentMessage, AgentTool, Model } from "@/agent/src/types/index.js";
import { findCutPoint } from "./cut-point.js";
import { estimateMessageTokens, estimatePayloadTokens } from "./token-estimator.js";
import { buildStructuralSummary } from "./structural-summary.js";
import { protectedRecentStart, shakeConversation } from "./shake.js";

export { protectedRecentStart, shakeConversation } from "./shake.js";

export * from "./token-estimator.js";
export * from "./cut-point.js";
export * from "./file-tracker.js";
export * from "./structural-summary.js";

export interface CompactionOptions {
  /** Enable automatic context window compaction. Default: true */
  enabled?: boolean;
  /** Maximum ratio of contextWindow before auto-compaction triggers. Default: 0.85 (85%) */
  maxThresholdRatio?: number;
  /** Keep the most recent N tokens uncompacted. Default: 20_000 (or 20% of context window) */
  keepRecentTokens?: number;
  /** Hard token threshold override. */
  tokenThreshold?: number;
  /** Keep at least this many recent conversation turns intact. Default: 3. */
  minimumRecentTurns?: number;
  /** Max characters allowed per tool result before truncation. Default: 8,000 */
  maxToolResultChars?: number;
  /** Max characters per tool result during emergency shaking. Default: 2,000 */
  emergencyToolResultChars?: number;
  /** Strategy for generating summary text. Default: "structural" */
  summaryStrategy?: "structural" | "llm";
}

export interface CompactionResult {
  compactedMessages: AgentMessage[];
  summary: string;
  originalCount: number;
  tokensBefore: number;
  tokensAfter: number;
}

/**
 * Determine if conversation history requires compaction.
 *
 * Pass full payload context (system prompt + tools) so the estimate covers
 * the actual wire payload instead of messages alone. Falls back to the
 * message-only heuristic when omitted.
 */
export function shouldCompact(
  messages: AgentMessage[],
  model: Model,
  options: CompactionOptions = {},
  payload?: { systemPrompt?: string; tools?: Pick<AgentTool, "name" | "description">[] },
): boolean {
  if (options.enabled === false) {
    return false;
  }

  const { maxThresholdRatio = 0.85, tokenThreshold } = options;
  const tokens = payload
    ? estimatePayloadTokens({ messages, ...payload }).tokens
    : estimateMessageTokens(messages);
  const limit = tokenThreshold ?? Math.floor(model.contextWindow * maxThresholdRatio);

  return tokens >= limit;
}

/**
 * Compact conversation history by summarizing older turns into a single checkpoint turn.
 */
export function compactHistory(
  messages: AgentMessage[],
  options: CompactionOptions = {},
): CompactionResult {
  const tokensBefore = estimateMessageTokens(messages);
  const keepRecent = options.keepRecentTokens ?? 40_000;
  const minimumRecentTurns = options.minimumRecentTurns ?? 3;
  const maxToolResultChars = options.maxToolResultChars ?? 8_000;
  // Short sessions have no summarizable history, but their tool outputs still
  // need ceilings — shake the whole history unprotected instead of sparing
  // every turn via the protected suffix.
  const isShortSession = messages.length <= 4;
  const shakenMessages = isShortSession
    ? shakeConversation(messages, maxToolResultChars, messages.length, true)
    : shakeConversation(
        messages,
        maxToolResultChars,
        protectedRecentStart(messages, minimumRecentTurns),
      );

  const { firstKeptIndex, isUserBoundary } = findCutPoint(shakenMessages, keepRecent, minimumRecentTurns);

  if (firstKeptIndex === 0 || messages.length <= 4) {
    return {
      compactedMessages: [...shakenMessages],
      summary: "History too short or cannot be safely partitioned.",
      originalCount: messages.length,
      tokensBefore,
      tokensAfter: estimateMessageTokens(shakenMessages),
    };
  }

  const olderMessages = shakenMessages.slice(0, firstKeptIndex);
  const recentMessages = shakenMessages.slice(firstKeptIndex);

  const summary = buildStructuralSummary(olderMessages);

  const summaryUserMessage: AgentMessage = {
    role: "user",
    content: summary,
  };

  let compactedMessages: AgentMessage[];

  if (isUserBoundary) {
    // [User summary, Assistant ack, User prompt, Assistant turn, ...]
    // Strictly alternates roles across providers.
    const summaryAssistantMessage: AgentMessage = {
      role: "assistant",
      id: crypto.randomUUID(),
      content: [
        {
          type: "text",
          text: "Understood. I have the context of prior work and files touched. Ready to proceed.",
        },
      ],
      stopReason: "stop",
    };

    compactedMessages = [
      summaryUserMessage,
      summaryAssistantMessage,
      ...recentMessages,
    ];
  } else {
    // If recentMessages begins with assistant, just user summary -> assistant
    compactedMessages = [
      summaryUserMessage,
      ...recentMessages,
    ];
  }

  const tokensAfter = estimateMessageTokens(compactedMessages);

  return {
    compactedMessages,
    summary,
    originalCount: messages.length,
    tokensBefore,
    tokensAfter,
  };
}
