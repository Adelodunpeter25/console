/**
 * Context Window Compaction Engine.
 * Monitors token usage, elides bloated tool results, and compacts old turns
 * into structured checkpoint summaries with cumulative file tracking.
 *
 * Inspired by oh-my-pi/packages/agent/src/compaction/.
 */
import crypto from "node:crypto";
import type { AgentMessage, Model } from "@/agent/src/types/index.js";
import { findCutPoint } from "./cut-point.js";
import { estimateMessageTokens } from "./token-estimator.js";
import { buildStructuralSummary } from "./structural-summary.js";

export * from "./token-estimator.js";
export * from "./cut-point.js";
export * from "./file-tracker.js";
export * from "./structural-summary.js";

export interface CompactionOptions {
  /** Enable automatic context window compaction. Default: true */
  enabled?: boolean;
  /** Maximum ratio of contextWindow before auto-compaction triggers. Default: 0.8 (80%) */
  maxThresholdRatio?: number;
  /** Keep the most recent N tokens uncompacted. Default: 20_000 (or 20% of context window) */
  keepRecentTokens?: number;
  /** Hard token threshold override. */
  tokenThreshold?: number;
  /** Max characters allowed per tool result before truncation. Default: 8,000 */
  maxToolResultChars?: number;
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
 */
export function shouldCompact(
  messages: AgentMessage[],
  model: Model,
  options: CompactionOptions = {},
): boolean {
  if (options.enabled === false) {
    return false;
  }

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
): CompactionResult {
  const tokensBefore = estimateMessageTokens(messages);
  const keepRecent = options.keepRecentTokens ?? 20_000;

  const { firstKeptIndex, isUserBoundary } = findCutPoint(messages, keepRecent);

  if (firstKeptIndex === 0 || messages.length <= 4) {
    return {
      compactedMessages: [...messages],
      summary: "History too short or cannot be safely partitioned.",
      originalCount: messages.length,
      tokensBefore,
      tokensAfter: tokensBefore,
    };
  }

  const olderMessages = messages.slice(0, firstKeptIndex);
  const recentMessages = messages.slice(firstKeptIndex);

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
