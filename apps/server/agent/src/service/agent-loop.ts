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
import { compactHistory, shouldCompact, type CompactionOptions } from "../compaction/index.js";
import { resolveApproval } from "../permissions/approval.js";
import { extractThinkingFromText } from "./thinking.js";
import { EventStream } from "./event-stream.js";
import type {
  AgentMessage,
  AgentSessionEvent,
  AgentTool,
  ApprovalMode,
  AssistantMessage,
  Model,
  PermissionRequest,
  ToolCall,
  ToolResult,
  ToolResultMessage,
  UserMessage,
} from "../types/index.js";

export type LLMDelta =
  | { type: "text"; text: string }
  | { type: "thinking"; text: string }
  | { type: "toolCall"; id: string; name: string; argumentsJson: string };

export type StreamFn = (params: {
  model: Model;
  systemPrompt: string;
  messages: AgentMessage[];
  tools: AgentTool[];
  signal?: AbortSignal;
}) => AsyncIterable<LLMDelta>;

export interface AgentLoopConfig {
  /** The model to use. */
  model: Model;
  /** System prompt sent on every request. */
  systemPrompt: string;
  /** Tools available to the agent. */
  tools: AgentTool[];
  /** Provider-specific streaming function — inject your Gemini/Antigravity client here. */
  streamFn: StreamFn;
  /** Security approval mode ("always-ask" | "accept-edits" | "plan-mode" | "full-access"). Default: "always-ask" */
  approvalMode?: ApprovalMode;
  /** Hook for user approval when a tool call requires permission. */
  onApproval?: (request: PermissionRequest) => Promise<boolean> | boolean;
  /**
   * Called for every AgentSessionEvent as it is emitted.
   * Useful for logging, UI updates, or WebSocket forwarding.
   */
  onEvent?: (event: AgentSessionEvent) => void;
  /** AbortSignal to cancel the run. */
  signal?: AbortSignal;
  /** Maximum number of tool-call turns before the loop stops. Default: 50. */
  maxTurns?: number;
  /** Compaction options for automated history summarization. */
  compaction?: CompactionOptions;
  /** Hook called before a tool is executed. Useful for approval flows. */
  onToolCall?: (call: ToolCall) => Promise<void> | void;
  /** Hook called after a tool finishes executing. */
  onToolResult?: (call: ToolCall, result: ToolResult) => Promise<void> | void;
}

interface StreamParams {
  model: Model;
  systemPrompt: string;
  messages: AgentMessage[];
  tools: AgentTool[];
  signal?: AbortSignal;
}

/**
 * Execute a single tool call with Zod parsing, Permission resolution, & error handling.
 */
async function executeTool(
  call: ToolCall,
  tools: AgentTool[],
  approvalMode: ApprovalMode,
  onApproval: AgentLoopConfig["onApproval"],
  emit: (event: AgentSessionEvent) => void,
  onToolCall?: AgentLoopConfig["onToolCall"],
  onToolResult?: AgentLoopConfig["onToolResult"],
): Promise<ToolResult> {
  const tool = tools.find((t) => t.name === call.name);

  if (!tool) {
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: `Tool "${call.name}" is not registered. Available tools: ${tools.map((t) => t.name).join(", ")}`,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }

  // Parse and validate arguments with Zod
  const parsed = tool.inputSchema.safeParse(call.arguments);
  if (!parsed.success) {
    const errorText = parsed.error.issues
      .map((issue) => `  - ${issue.path.join(".")}: ${issue.message}`)
      .join("\n");
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: `Invalid arguments for tool "${call.name}":\n${errorText}`,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }

  // Permissions & Approval resolution
  const approval = resolveApproval(tool, parsed.data, approvalMode);
  if (approval.policy === "deny") {
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: `Execution denied: ${approval.reason}`,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }

  if (approval.policy === "prompt") {
    const req: PermissionRequest = {
      requestId: randomUUID(),
      toolCallId: call.id,
      toolName: call.name,
      args: parsed.data,
      tier: approval.tier,
      reason: approval.reason,
    };
    emit({ type: "permissionRequest", request: req });

    if (onApproval) {
      const allowed = await onApproval(req);
      if (!allowed) {
        const result: ToolResult = {
          toolCallId: call.id,
          toolName: call.name,
          content: `Execution denied by user permission decision.`,
          isError: true,
        };
        await onToolResult?.(call, result);
        return result;
      }
    }
  }

  try {
    await onToolCall?.(call);
    const output = await tool.execute(parsed.data);
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: output,
    };
    await onToolResult?.(call, result);
    return result;
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    const result: ToolResult = {
      toolCallId: call.id,
      toolName: call.name,
      content: message,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }
}

/**
 * Stream one turn from the LLM, collecting all text, thinking, and tool-call deltas.
 */
async function streamOneTurn(
  params: StreamParams,
  streamFn: StreamFn,
  turnId: string,
  emit: (event: AgentSessionEvent) => void,
): Promise<AssistantMessage> {
  emit({ type: "modelStreamStart", turnId });

  let textAccumulator = "";
  let thinkingAccumulator = "";
  const toolCallMap = new Map<string, { id: string; name: string; argumentsJson: string }>();
  const toolCallOrder: string[] = [];

  const stream = streamFn(params);
  for await (const delta of stream) {
    if (params.signal?.aborted) break;

    if (delta.type === "text") {
      textAccumulator += delta.text;
      emit({ type: "modelStreamPart", part: { text: delta.text } });
    } else if (delta.type === "thinking") {
      thinkingAccumulator += delta.text;
      emit({ type: "modelStreamPart", part: { thinking: delta.text } });
    } else if (delta.type === "toolCall") {
      const existing = toolCallMap.get(delta.id);
      if (existing) {
        existing.argumentsJson += delta.argumentsJson;
      } else {
        toolCallMap.set(delta.id, {
          id: delta.id,
          name: delta.name,
          argumentsJson: delta.argumentsJson,
        });
        toolCallOrder.push(delta.id);
        emit({
          type: "modelStreamPart",
          part: { toolCall: { id: delta.id, name: delta.name, arguments: undefined } },
        });
      }
    }
  }

  const toolCalls: ToolCall[] = toolCallOrder.map((id) => {
    const tc = toolCallMap.get(id)!;
    let args: unknown;
    try {
      args = tc.argumentsJson ? JSON.parse(tc.argumentsJson) : {};
    } catch {
      args = {};
    }
    return { id: tc.id, name: tc.name, arguments: args };
  });

  const content: AssistantMessage["content"] = [];

  // 1. Direct thinking deltas
  if (thinkingAccumulator) {
    content.push({ type: "thinking", text: thinkingAccumulator });
  }

  // 2. Extract <thinking>...</thinking> tags from text accumulator
  if (textAccumulator) {
    const { textParts, thinkingParts } = extractThinkingFromText(textAccumulator);
    content.push(...thinkingParts);
    content.push(...textParts);
  }

  // 3. Tool calls
  for (const tc of toolCalls) {
    content.push({ type: "toolCall", call: tc });
  }

  const stopReason: AssistantMessage["stopReason"] = toolCalls.length > 0 ? "toolUse" : "stop";

  return {
    role: "assistant",
    id: turnId,
    content,
    stopReason,
  };
}

/**
 * Centralized agentic turn loop execution core.
 */
function runAgentLoop(
  prompt: string,
  config: AgentLoopConfig,
  initialMessages: AgentMessage[] = [],
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

      const userMessage: UserMessage = { role: "user", content: prompt };
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
            executeTool(call, tools, approvalMode, onApproval, emit, onToolCall, onToolResult).then(
              (result) => {
                emit({ type: "toolExecutionResult", result });
                return result;
              },
            ),
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
        const message = err instanceof Error ? err.message : String(err);
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
): EventStream<AgentSessionEvent, AgentMessage[]> {
  return runAgentLoop(prompt, config, []);
}

/**
 * Continue an existing session with a new prompt and prior history.
 */
export function agentLoopContinue(
  priorMessages: AgentMessage[],
  prompt: string,
  config: AgentLoopConfig,
): EventStream<AgentSessionEvent, AgentMessage[]> {
  return runAgentLoop(prompt, config, priorMessages);
}
