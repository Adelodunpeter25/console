import type { AgentMessage } from "@/agent/src/types/index.js";
import { estimateMessageTokens } from "./token-estimator.js";

export interface CutPointResult {
  /** Index of the first message to keep in the active context. */
  firstKeptIndex: number;
  /** Whether the cut point successfully landed at a clean user boundary. */
  isUserBoundary: boolean;
}

/**
 * Determine whether slicing at `candidateIndex` would orphan any toolResult
 * from its corresponding assistant toolCall.
 */
export function isToolCallSafe(messages: AgentMessage[], candidateIndex: number): boolean {
  if (candidateIndex <= 0 || candidateIndex >= messages.length) {
    return true;
  }

  // Never cut at a toolResult
  if (messages[candidateIndex].role === "toolResult") {
    return false;
  }

  // Collect all tool call IDs produced in the discarded prefix (0 .. candidateIndex - 1)
  const discardedToolCallIds = new Set<string>();
  for (let i = 0; i < candidateIndex; i++) {
    const msg = messages[i];
    if (msg.role === "assistant") {
      for (const part of msg.content) {
        if (part.type === "toolCall") {
          discardedToolCallIds.add(part.call.id);
        }
      }
    }
  }

  // Ensure no kept message in (candidateIndex .. end) is a toolResult for a discarded call
  for (let i = candidateIndex; i < messages.length; i++) {
    const msg = messages[i];
    if (msg.role === "toolResult") {
      for (const res of msg.results) {
        if (discardedToolCallIds.has(res.toolCallId)) {
          return false;
        }
      }
    }
  }

  return true;
}

/**
 * Find the optimal cut point in `messages` that preserves at least `keepRecentTokens`
 * while strictly adhering to turn-safety and provider role invariants.
 */
export function findCutPoint(
  messages: AgentMessage[],
  keepRecentTokens: number = 20_000,
): CutPointResult {
  if (messages.length <= 4) {
    return { firstKeptIndex: 0, isUserBoundary: true };
  }

  // Find all user turn indices
  const userIndices: number[] = [];
  for (let i = 0; i < messages.length; i++) {
    if (messages[i].role === "user") {
      userIndices.push(i);
    }
  }

  // Walk backwards from newest, accumulating token count
  let accumulatedTokens = 0;
  let targetIndex = 0;

  for (let i = messages.length - 1; i >= 0; i--) {
    accumulatedTokens += estimateMessageTokens([messages[i]]);
    if (accumulatedTokens >= keepRecentTokens) {
      targetIndex = i;
      break;
    }
  }

  // Preference 1: Nearest user turn at or after targetIndex (to ensure we preserve enough tokens,
  // or right before targetIndex if targetIndex is after the last user turn).
  // Look for a user turn that gives a safe cut point.
  let bestCutIndex = -1;

  // Search user turns from newest to oldest
  for (let u = userIndices.length - 1; u >= 0; u--) {
    const idx = userIndices[u];
    // We want a user turn such that we don't discard the whole conversation (idx > 0)
    if (idx > 0 && idx >= targetIndex && isToolCallSafe(messages, idx)) {
      bestCutIndex = idx;
      break;
    }
  }

  // If no user turn at or after targetIndex was found, try the closest user turn before targetIndex
  if (bestCutIndex === -1) {
    for (let u = userIndices.length - 1; u >= 1; u--) {
      const idx = userIndices[u];
      if (isToolCallSafe(messages, idx)) {
        bestCutIndex = idx;
        break;
      }
    }
  }

  if (bestCutIndex > 0) {
    return { firstKeptIndex: bestCutIndex, isUserBoundary: true };
  }

  // Preference 2: If the session has only 1 user turn (e.g. 50 tool turns in one prompt),
  // walk backwards from targetIndex and find the first tool-call safe assistant message.
  for (let i = targetIndex; i < messages.length - 2; i++) {
    if (messages[i].role === "assistant" && isToolCallSafe(messages, i)) {
      return { firstKeptIndex: i, isUserBoundary: false };
    }
  }

  // Fallback: cannot safely compact without breaking turn pairing
  return { firstKeptIndex: 0, isUserBoundary: true };
}
