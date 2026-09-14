/**
 * Core Agentic Turn-and-Tool Loop.
 *
 * Implements the stateful conversation cycle:
 *  1. Format prompt + prior history into messages
 *  2. Check and perform auto-compaction if history exceeds context threshold
 *  3. Call injected StreamFn transport to stream LLM response deltas (text, thinking, toolCalls)
 *  4. Handle tool call requests with Zod validation & Permission Approval check
 *  5. Execute tools concurrently (Promise.all)
 *  6. Add assistant turn & tool results to history
 *  7. Loop until stopReason === 'stop' or signal aborted
 */
import { randomUUID } from "node:crypto";
import { compactHistory, estimateMessageTokens, shouldCompact } from "@/agent/src/compaction/index.js";
import { shakeConversation, EMERGENCY_TOOL_RESULT_MAX_CHARS } from "@/agent/src/compaction/shake.js";
import { estimatePayloadTokens } from "@/agent/src/compaction/token-estimator.js";
import {
  DEFAULT_TOOL_RESULT_MAX_CHARS,
  truncateMessageToolResults,
  truncateToolResultContent,
} from "../utils/text-truncate.js";
import { EventStream } from "./event-stream.js";
import { executeTool } from "./tool-executor.js";
import { streamOneTurn } from "./stream-turn.js";
import { extractErrorMessage, isContextOverflowError } from "@/agent/src/utils/error.js";
import type {
  AgentMessage,
  AgentSessionEvent,
  ImagePart,
  ToolResultMessage,
  UserMessage,
} from "@/agent/src/types/index.js";
import type { AgentLoopConfig } from "./types.js";

export type {
  LLMDelta,
  StreamFn,
  StreamParams,
  AgentLoopConfig,
} from "./types.js";
export { executeTool } from "./tool-executor.js";
export { streamOneTurn } from "./stream-turn.js";

/**
 * Centralized agentic turn loop execution core.
 */
function compactHistoryWithSummary(
  messages: AgentMessage[],
  options: import("@/agent/src/compaction/index.js").CompactionOptions,
  summary: string,
) {
  const structural = compactHistory(messages, options);
  if (structural.compactedMessages.length === messages.length) return structural;
  const firstUserIndex = structural.compactedMessages.findIndex((message) => message.role === "user");
  if (firstUserIndex < 0) return structural;
  const summaryUserMessage: AgentMessage = { role: "user", content: summary };
  const rest = structural.compactedMessages.slice(firstUserIndex + 1);
  const compactedMessages = [summaryUserMessage, ...rest];
  return {
    ...structural,
    compactedMessages,
    summary,
    tokensAfter: estimateMessageTokens(compactedMessages),
  };
}

function runAgentLoop(
  prompt: string,
  config: AgentLoopConfig,
  initialMessages: AgentMessage[] = [],
  attachments: ImagePart[] = [],
): EventStream<AgentSessionEvent, AgentMessage[]> {
  const {
    model,
    systemPrompt,
    tools,
    streamFn,
    thinkingLevel,
    approvalMode = "always-ask",
    onApproval,
    onEvent,
    signal,
    compaction,
    onToolCall,
    onToolResult,
    summarizeCompaction,
    cacheRetention,
    cacheIdentity,
  } = config;

  const stream = new EventStream<AgentSessionEvent, AgentMessage[]>(
    (e) => e.type === "sessionEnd",
    () => messages,
  );

  const maxToolChars = compaction?.maxToolResultChars ?? DEFAULT_TOOL_RESULT_MAX_CHARS;
  const messages: AgentMessage[] = initialMessages.map((m) =>
    truncateMessageToolResults(m, maxToolChars),
  );

  const emit = (event: AgentSessionEvent) => {
    onEvent?.(event);
    stream.push(event);
  };

  (async () => {
    try {
      emit({ type: "sessionStart" });
      emit({ type: "turnStart", prompt });

      const userMessage: UserMessage =
        attachments.length > 0
          ? { role: "user", content: prompt, attachments }
          : { role: "user", content: prompt };
      messages.push(userMessage);

      while (true) {
        if (signal?.aborted) {
          // User-initiated abort is normal control flow, not an error.
          // Just break the loop — sessionEnd is emitted in finally.
          break;
        }

        // Auto-compaction check (payload-aware: system + tools + history)
        if (compaction && shouldCompact(messages, model, compaction, { systemPrompt, tools })) {
          let compactionResult = compactHistory(messages, compaction);
          if (compaction.summaryStrategy === "llm" && summarizeCompaction) {
            try {
              const summary = await summarizeCompaction(messages, signal);
              if (summary.trim()) {
                compactionResult = compactHistoryWithSummary(messages, compaction, summary);
              }
            } catch {
              // Structural compaction remains the safe fallback.
            }
          }
          const { compactedMessages, summary, originalCount, tokensBefore, tokensAfter } = compactionResult;
          messages.length = 0;
          messages.push(...compactedMessages);
          emit({
            type: "compaction",
            summary,
            originalMessageCount: originalCount,
            compactedMessageCount: compactedMessages.length,
            tokensBefore,
            tokensAfter,
            compactedMessages: [...compactedMessages],
            tier: "summarize",
            trigger: "pre_turn",
          });
        }

        const turnId = randomUUID();

        // Stream the turn with overflow recovery: a context-overflow 400
        // triggers emergency compaction and exactly one retry of the turn.
        // A second overflow (or any other error) propagates to the session
        // error handler below.
        let assistantMessage;
        let overflowRetried = false;
        for (;;) {
          try {
            assistantMessage = await streamOneTurn(
              { model, systemPrompt, messages: [...messages], tools, signal, thinkingLevel, cacheRetention, cacheIdentity },
              streamFn,
              turnId,
              emit,
            );
            break;
          } catch (err) {
            if (overflowRetried || signal?.aborted || !compaction || !isContextOverflowError(err)) {
              throw err;
            }
            overflowRetried = true;
            const emergencyChars = compaction.emergencyToolResultChars ?? EMERGENCY_TOOL_RESULT_MAX_CHARS;
            const before = estimatePayloadTokens({ messages, systemPrompt, tools });
            const shaken = shakeConversation(messages, emergencyChars, messages.length, true);
            messages.length = 0;
            messages.push(...shaken);
            const recovery = compactHistory(messages, { ...compaction, keepRecentTokens: 20_000 });
            messages.length = 0;
            messages.push(...recovery.compactedMessages);
            const after = estimatePayloadTokens({ messages, systemPrompt, tools });
            emit({
              type: "compaction",
              summary: `Emergency overflow recovery (${extractErrorMessage(err)})`,
              originalMessageCount: recovery.originalCount,
              compactedMessageCount: messages.length,
              tokensBefore: before.tokens,
              tokensAfter: after.tokens,
              compactedMessages: [...messages],
              tier: "emergency_recovery",
              trigger: "overflow_retry",
              tokenCountSource: before.source,
            });
          }
        }

        emit({ type: "modelStreamEnd", turnId, turn: assistantMessage });
        messages.push(assistantMessage);

        const toolCalls = assistantMessage.content
          .filter((c): c is Extract<typeof c, { type: "toolCall" }> => c.type === "toolCall")
          .map((c) => c.call);

        if (toolCalls.length === 0 || assistantMessage.stopReason === "stop") {
          emit({ type: "turnEnd", turnId });
          break;
        }

        emit({ type: "toolExecutionStart", calls: toolCalls });

        const results = await Promise.all(
          toolCalls.map((call) =>
            executeTool(
              call,
              tools,
              approvalMode,
              onApproval,
              emit,
              onToolCall,
              onToolResult,
              signal,
            ).then((result) => {
              emit({ type: "toolExecutionResult", result });
              return result;
            }),
          ),
        );

        const boundedResults = results.map((r) => ({
          ...r,
          content: truncateToolResultContent(r.content, maxToolChars),
        }));

        const toolResultMessage: ToolResultMessage = {
          role: "toolResult",
          results: boundedResults,
        };
        messages.push(toolResultMessage);
        emit({ type: "toolExecutionEnd", results: boundedResults });

        // Mid-turn budget check: bloated tool outputs can overflow the very
        // next model call, so shake before continuing the turn.
        if (compaction && shouldCompact(messages, model, compaction, { systemPrompt, tools })) {
          const before = estimatePayloadTokens({ messages, systemPrompt, tools });
          const shaken = shakeConversation(messages, maxToolChars);
          messages.length = 0;
          messages.push(...shaken);
          const after = estimatePayloadTokens({ messages, systemPrompt, tools });
          emit({
            type: "compaction",
            summary: "Mechanical mid-turn shake of bloated tool outputs.",
            originalMessageCount: messages.length,
            compactedMessageCount: messages.length,
            tokensBefore: before.tokens,
            tokensAfter: after.tokens,
            compactedMessages: [...messages],
            tier: "shake",
            trigger: "mid_turn",
            tokenCountSource: before.source,
          });
        }

        emit({ type: "turnEnd", turnId });
      }
    } catch (err) {
      // User-initiated abort is normal control flow — don't surface as error.
      const isAbort =
        signal?.aborted ||
        (err instanceof Error && err.name === "AbortError") ||
        (err instanceof Error && err.message === "This operation was aborted.");
      if (!isAbort) {
        const message = extractErrorMessage(err);
        emit({ type: "error", error: { message } });
      }
    } finally {
      emit({ type: "sessionEnd" });
    }
  })();

  return stream;
}

/**
 * Start a new agent session for a given prompt.
 */
export function agentLoop(
  prompt: string,
  config: AgentLoopConfig,
  attachments?: ImagePart[],
): EventStream<AgentSessionEvent, AgentMessage[]> {
  return runAgentLoop(prompt, config, [], attachments);
}

/**
 * Continue an existing session with a new prompt and prior history.
 */
export function agentLoopContinue(
  priorMessages: AgentMessage[],
  prompt: string,
  config: AgentLoopConfig,
  attachments?: ImagePart[],
): EventStream<AgentSessionEvent, AgentMessage[]> {
  return runAgentLoop(prompt, config, priorMessages, attachments);
}
