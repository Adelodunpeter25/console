/**
 * Canonical conversation history transformer for Claude and Anthropic-compatible providers.
 *
 * Ensures:
 * 1. Every assistant toolCall is immediately followed by a matching toolResult.
 *    Interrupted, errored, or aborted tool calls get a synthetic error result.
 * 2. Orphaned tool results (lacking a preceding toolCall) become safe user text.
 * 3. Truncated thinking-only assistant turns are pruned.
 * 4. Trailing whitespace on assistant text / demoted thinking is trimmed.
 * 5. Conversations strictly start and end with a user turn (prevents
 *    "This model does not support assistant message prefill" on thinking models).
 * 6. Same-role turns are merged to satisfy strict role alternation.
 */
import type {
  AgentMessage,
  AssistantMessage,
  AssistantMessageContent,
  ToolCall,
  ToolResult,
  ToolResultMessage,
  UserMessage,
} from "@console/types";

export const INTERRUPTED_TOOL_RESULT_TEXT =
  "Tool execution was interrupted before a result was recorded.";

export const DEFAULT_CONTINUE_PROMPT = "Continue.";
export const DEFAULT_SESSION_START_PROMPT = "(session started)";

function isAssistantMessage(msg: AgentMessage): msg is AssistantMessage {
  return msg.role === "assistant";
}

function isUserMessage(msg: AgentMessage): msg is UserMessage {
  return msg.role === "user";
}

function isToolResultMessage(msg: AgentMessage): msg is ToolResultMessage {
  return msg.role === "toolResult";
}

function extractToolCalls(msg: AssistantMessage): ToolCall[] {
  return msg.content
    .filter((p): p is Extract<typeof p, { type: "toolCall" }> => p.type === "toolCall")
    .map((p) => p.call);
}

function makeSyntheticResult(call: ToolCall): ToolResult {
  return {
    toolCallId: call.id,
    toolName: call.name,
    content: INTERRUPTED_TOOL_RESULT_TEXT,
    isError: true,
  };
}

export interface TransformMessagesOptions {
  /**
   * Whether to demote assistant `thinking` blocks into `text` blocks.
   * Defaults to true (Anthropic signing endpoints reject replayed unsigned thinking).
   */
  demoteThinkingToText?: boolean;
  /**
   * Prompt used to close a conversation that terminates on an assistant turn.
   * Defaults to "Continue.".
   */
  continuationPrompt?: string;
  /**
   * Prompt used to prefix a conversation that begins on an assistant turn.
   * Defaults to "(session started)".
   */
  sessionStartPrompt?: string;
}

/**
 * Transforms AgentMessage[] into a safe, normalized history compliant with
 * Anthropic Messages and Claude CCA API requirements.
 */
export function transformMessages(
  messages: AgentMessage[],
  options: TransformMessagesOptions = {},
): AgentMessage[] {
  const {
    demoteThinkingToText = true,
    continuationPrompt = DEFAULT_CONTINUE_PROMPT,
    sessionStartPrompt = DEFAULT_SESSION_START_PROMPT,
  } = options;

  if (messages.length === 0) {
    return [{ role: "user", content: continuationPrompt }];
  }

  // 1. First pass: normalize assistant content (trim whitespace, demote thinking if requested)
  // and drop truncated thinking-only assistant messages with no actionable content.
  const sanitizedInput: AgentMessage[] = [];
  for (const msg of messages) {
    if (isAssistantMessage(msg)) {
      const hasOriginalText = msg.content.some((p) => p.type === "text" && p.text.trim() !== "");
      const hasOriginalToolCall = msg.content.some((p) => p.type === "toolCall");
      const isTruncatedThinkingOnly =
        !hasOriginalText &&
        !hasOriginalToolCall &&
        (msg.stopReason === "aborted" ||
          msg.stopReason === "maxTokens" ||
          (msg as { stopReason?: string }).stopReason === "error");

      if (isTruncatedThinkingOnly) {
        continue;
      }

      const newContent: AssistantMessageContent[] = [];

      for (const part of msg.content) {
        if (part.type === "text") {
          const text = part.text.trimEnd();
          if (text) {
            newContent.push({ ...part, text });
          }
        } else if (part.type === "thinking") {
          const text = part.text.trimEnd();
          if (text) {
            if (demoteThinkingToText) {
              newContent.push({ type: "text", text });
            } else {
              newContent.push({ ...part, text });
            }
          }
        } else if (part.type === "toolCall") {
          newContent.push(part);
        }
      }

      if (newContent.length === 0) {
        continue;
      }

      sanitizedInput.push({
        ...msg,
        content: newContent,
      });
    } else {
      sanitizedInput.push(msg);
    }
  }

  // 2. Second pass: ensure every toolCall is followed immediately by a toolResult.
  // Synthesize missing results for aborted/interrupted tool calls.
  // Convert orphaned tool results into safe user text blocks.
  const pairedMessages: AgentMessage[] = [];
  const knownToolCallIds = new Set<string>();

  for (let i = 0; i < sanitizedInput.length; i++) {
    const msg = sanitizedInput[i]!;

    if (isAssistantMessage(msg)) {
      const calls = extractToolCalls(msg);
      for (const c of calls) {
        knownToolCallIds.add(c.id);
      }
      pairedMessages.push(msg);

      if (calls.length === 0) {
        continue;
      }

      // Check if next message is a toolResult
      const nextMsg = sanitizedInput[i + 1];
      if (nextMsg && isToolResultMessage(nextMsg)) {
        // Next message is a tool result. Check if all calls are answered.
        const answeredIds = new Set(nextMsg.results.map((r) => r.toolCallId));
        const missingCalls = calls.filter((c) => !answeredIds.has(c.id));

        if (missingCalls.length > 0) {
          // Augment with synthetic results for the missing calls
          pairedMessages.push({
            ...nextMsg,
            results: [...nextMsg.results, ...missingCalls.map(makeSyntheticResult)],
          });
        } else {
          pairedMessages.push(nextMsg);
        }
        i++; // skip nextMsg as it has been consumed
      } else {
        // No toolResult follows — synthesize one immediately
        pairedMessages.push({
          role: "toolResult",
          results: calls.map(makeSyntheticResult),
        });
      }
    } else if (isToolResultMessage(msg)) {
      // Tool result that was not consumed by the preceding assistant message.
      // Check if its toolCallIds were ever defined by a preceding assistant message.
      const validResults: ToolResult[] = [];
      const orphanResults: ToolResult[] = [];

      for (const res of msg.results) {
        if (knownToolCallIds.has(res.toolCallId)) {
          validResults.push(res);
        } else {
          orphanResults.push(res);
        }
      }

      if (validResults.length > 0) {
        pairedMessages.push({ ...msg, results: validResults });
      }

      if (orphanResults.length > 0) {
        // Convert orphaned results into readable text in a user message
        const textParts = orphanResults.map((r) => {
          const contentStr =
            typeof r.content === "string" ? r.content : JSON.stringify(r.content ?? "");
          return `[Tool result for ${r.toolName || r.toolCallId}: ${contentStr}]`;
        });
        pairedMessages.push({
          role: "user",
          content: textParts.join("\n"),
        });
      }
    } else {
      pairedMessages.push(msg);
    }
  }

  // 3. Third pass: merge adjacent same-role turns to satisfy strict alternation.
  // Note: toolResult will be serialized as role: "user" in Anthropic/Claude, so
  // consecutive [user, toolResult] or [toolResult, user] or [user, user]
  // need consistent handling. In AgentMessage, UserMessage and ToolResultMessage
  // are separate roles; we keep them clean here and merge adjacent users / assistants.
  const merged: AgentMessage[] = [];
  for (const msg of pairedMessages) {
    const last = merged[merged.length - 1];

    if (last && last.role === "user" && isUserMessage(msg)) {
      // Merge adjacent user messages
      const combinedText = [last.content, msg.content].filter(Boolean).join("\n\n");
      const combinedAttachments = [
        ...(last.attachments ?? []),
        ...(msg.attachments ?? []),
      ];
      last.content = combinedText;
      if (combinedAttachments.length > 0) {
        last.attachments = combinedAttachments;
      }
    } else if (last && last.role === "assistant" && isAssistantMessage(msg)) {
      // Merge adjacent assistant messages
      last.content = [...last.content, ...msg.content];
      if (msg.stopReason) last.stopReason = msg.stopReason;
    } else {
      merged.push(msg);
    }
  }

  // 4. Fourth pass: ensure conversation starts and ends with a user turn.
  if (merged.length === 0) {
    return [{ role: "user", content: continuationPrompt }];
  }

  // Starts with assistant -> prepend user starter
  if (merged[0]!.role === "assistant") {
    merged.unshift({ role: "user", content: sessionStartPrompt });
  }

  // Ends with assistant -> append continuation prompt
  const lastMsg = merged[merged.length - 1]!;
  if (lastMsg.role === "assistant") {
    const calls = extractToolCalls(lastMsg);
    if (calls.length > 0) {
      // Assistant ended with tool calls: append toolResult first, then user continuation
      merged.push({
        role: "toolResult",
        results: calls.map(makeSyntheticResult),
      });
      merged.push({ role: "user", content: continuationPrompt });
    } else {
      merged.push({ role: "user", content: continuationPrompt });
    }
  }

  return merged;
}
