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
 *  7. Loop until stopReason === 'stop' or maxTurns reached
 */
import { randomUUID } from "node:crypto";
import { compactHistory, shouldCompact } from "../compaction/index.js";
import { EventStream } from "./event-stream.js";
import { executeTool } from "./tool-executor.js";
import { streamOneTurn } from "./stream-turn.js";
import { extractErrorMessage } from "../utils/error.js";
import type {
  AgentMessage,
  AgentSessionEvent,
  ImagePart,
  ToolResultMessage,
  UserMessage,
} from "../types/index.js";
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
    approvalMode = "always-ask",
    onApproval,
    onEvent,
    signal,
    maxTurns = 50,
    compaction,
    onToolCall,
    onToolResult,
  } = config;

  const stream = new EventStream<AgentSessionEvent, AgentMessage[]>(
    (e) => e.type === "sessionEnd",
    () => messages,
  );

  const messages: AgentMessage[] = [...initialMessages];

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

      let turnCount = 0;

      while (true) {
        if (signal?.aborted) {
          // User-initiated abort is normal control flow, not an error.
          // Just break the loop — sessionEnd is emitted in finally.
          break;
        }

        if (turnCount >= maxTurns) {
          emit({
            type: "error",
            error: { message: `Maximum turns reached (${maxTurns}). Stopping loop.` },
          });
          break;
        }

        // Auto-compaction check
        if (compaction && shouldCompact(messages, model, compaction)) {
          const { compactedMessages, summary, originalCount } = compactHistory(
            messages,
            compaction,
          );
          messages.length = 0;
          messages.push(...compactedMessages);
          emit({ type: "compaction", summary, originalMessageCount: originalCount });
        }

        turnCount++;
        const turnId = randomUUID();

        const assistantMessage = await streamOneTurn(
          { model, systemPrompt, messages: [...messages], tools, signal },
          streamFn,
          turnId,
          emit,
        );

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

        const toolResultMessage: ToolResultMessage = {
          role: "toolResult",
          results,
        };
        messages.push(toolResultMessage);
        emit({ type: "toolExecutionEnd", results });
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
