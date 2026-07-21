import { randomUUID } from "node:crypto";
import type {
  AgentMessage,
  AgentTool,
  AssistantMessage,
  Model,
  SessionContext,
  AgentSessionEvent,
  ToolCall,
  ToolResult,
  ToolResultMessage,
  UserMessage,
} from "../types/index.js";
import { EventStream } from "./event-stream.js";

// ---------------------------------------------------------------------------
// StreamFn — the pluggable LLM streaming interface
// ---------------------------------------------------------------------------

/** A single streaming delta from the LLM */
export type LLMDelta =
  | { type: "text"; text: string }
  | { type: "toolCall"; id: string; name: string; argumentsJson: string };

/** Parameters passed to the streaming function */
export interface StreamParams {
  model: Model;
  systemPrompt: string;
  messages: AgentMessage[];
  tools: AgentTool[];
  signal?: AbortSignal;
}

/**
 * A provider-agnostic streaming function. Yields deltas from the LLM.
 * Inject this into AgentLoopConfig — the loop itself is provider-free.
 */
export type StreamFn = (params: StreamParams) => AsyncIterable<LLMDelta>;

// ---------------------------------------------------------------------------
// AgentLoopConfig
// ---------------------------------------------------------------------------

export interface AgentLoopConfig {
  /** The model to use. */
  model: Model;
  /** System prompt sent on every request. */
  systemPrompt: string;
  /** Tools available to the agent. */
  tools: AgentTool[];
  /** Provider-specific streaming function — inject your Gemini/Antigravity client here. */
  streamFn: StreamFn;
  /**
   * Called for every AgentSessionEvent as it is emitted.
   * Useful for logging, UI updates, or WebSocket forwarding.
   */
  onEvent?: (event: AgentSessionEvent) => void;
  /** AbortSignal to cancel the run. */
  signal?: AbortSignal;
  /** Maximum number of tool-call turns before the loop stops. Default: 50. */
  maxTurns?: number;
  /** Hook called before a tool is executed. Useful for approval flows. */
  onToolCall?: (call: ToolCall) => Promise<void> | void;
  /** Hook called after a tool finishes executing. */
  onToolResult?: (call: ToolCall, result: ToolResult) => Promise<void> | void;
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/**
 * Execute a single tool call against the tool registry.
 * Never throws — all errors are captured into isError ToolResult.
 */
async function executeTool(
  call: ToolCall,
  tools: AgentTool[],
  onToolCall?: AgentLoopConfig["onToolCall"],
  onToolResult?: AgentLoopConfig["onToolResult"],
): Promise<ToolResult> {
  const tool = tools.find((t) => t.name === call.name);

  if (!tool) {
    const result: ToolResult = {
      toolCallId: call.id,
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
      content: `Invalid arguments for tool "${call.name}":\n${errorText}`,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }

  try {
    await onToolCall?.(call);
    const output = await tool.execute(parsed.data);
    const result: ToolResult = {
      toolCallId: call.id,
      content: output,
    };
    await onToolResult?.(call, result);
    return result;
  } catch (err) {
    const message =
      err instanceof Error ? err.message : String(err);
    const result: ToolResult = {
      toolCallId: call.id,
      content: message,
      isError: true,
    };
    await onToolResult?.(call, result);
    return result;
  }
}

/**
 * Stream one turn from the LLM, collecting all text and tool-call deltas.
 * Emits streaming events through the provided emit callback.
 */
async function streamOneTurn(
  params: StreamParams,
  streamFn: StreamFn,
  turnId: string,
  emit: (event: AgentSessionEvent) => void,
): Promise<AssistantMessage> {
  emit({ type: "modelStreamStart", turnId });

  let textAccumulator = "";
  // Map from toolCall id → accumulated state
  const toolCallMap = new Map<
    string,
    { id: string; name: string; argumentsJson: string }
  >();
  // Track insertion order
  const toolCallOrder: string[] = [];

  const stream = streamFn(params);
  for await (const delta of stream) {
    if (params.signal?.aborted) break;

    if (delta.type === "text") {
      textAccumulator += delta.text;
      emit({ type: "modelStreamPart", part: { text: delta.text } });
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
        emit({ type: "modelStreamPart", part: { toolCall: { id: delta.id, name: delta.name, arguments: undefined } } });
      }
    }
  }

  // Build the tool calls list in insertion order
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
  if (textAccumulator) {
    content.push({ type: "text", text: textAccumulator });
  }
  for (const tc of toolCalls) {
    content.push({ type: "toolCall", call: tc });
  }

  const stopReason: AssistantMessage["stopReason"] =
    toolCalls.length > 0 ? "toolUse" : "stop";

  const assistantMessage: AssistantMessage = {
    role: "assistant",
    id: turnId,
    content,
    stopReason,
  };

  return assistantMessage;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Start the agent agentic loop for a given prompt.
 *
 * Returns an EventStream that emits AgentSessionEvent objects.
 * Resolves with the complete AgentMessage[] history when the session ends.
 */
export function agentLoop(
  prompt: string,
  config: AgentLoopConfig,
): EventStream<AgentSessionEvent, AgentMessage[]> {
  const {
    model,
    systemPrompt,
    tools,
    streamFn,
    onEvent,
    signal,
    maxTurns = 50,
    onToolCall,
    onToolResult,
  } = config;

  const stream = new EventStream<AgentSessionEvent, AgentMessage[]>(
    (e) => e.type === "sessionEnd",
    () => messages,
  );

  const messages: AgentMessage[] = [];

  const emit = (event: AgentSessionEvent) => {
    onEvent?.(event);
    stream.push(event);
  };

  // Run the loop asynchronously
  (async () => {
    try {
      // Emit session start
      emit({ type: "sessionStart" });
      emit({ type: "turnStart", prompt });

      // Add initial user message
      const userMessage: UserMessage = { role: "user", content: prompt };
      messages.push(userMessage);

      let turnCount = 0;

      while (true) {
        // Check abort
        if (signal?.aborted) {
          emit({ type: "error", error: { message: "Run was aborted." } });
          break;
        }

        // Check max turns
        if (turnCount >= maxTurns) {
          emit({
            type: "error",
            error: {
              message: `Maximum turns reached (${maxTurns}). Stopping the agent loop.`,
            },
          });
          break;
        }

        turnCount++;
        const turnId = randomUUID();

        // Stream one turn from the LLM
        const assistantMessage = await streamOneTurn(
          { model, systemPrompt, messages: [...messages], tools, signal },
          streamFn,
          turnId,
          emit,
        );

        emit({ type: "modelStreamEnd", turn: assistantMessage });
        messages.push(assistantMessage);

        // Extract tool calls from content
        const toolCalls = assistantMessage.content
          .filter((c): c is Extract<typeof c, { type: "toolCall" }> => c.type === "toolCall")
          .map((c) => c.call);

        // If no tool calls, the agent has finished
        if (toolCalls.length === 0 || assistantMessage.stopReason === "stop") {
          emit({ type: "turnEnd", turnId });
          break;
        }

        // Execute all tool calls concurrently
        emit({ type: "toolExecutionStart", calls: toolCalls });

        const results = await Promise.all(
          toolCalls.map((call) =>
            executeTool(call, tools, onToolCall, onToolResult).then((result) => {
              emit({ type: "toolExecutionResult", result });
              return result;
            }),
          ),
        );

        // Add tool result message to history
        const toolResultMessage: ToolResultMessage = {
          role: "toolResult",
          results,
        };
        messages.push(toolResultMessage);

        emit({ type: "turnEnd", turnId });
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      emit({ type: "error", error: { message } });
      stream.fail(err);
      return;
    }

    // Always emit sessionEnd last
    emit({ type: "sessionEnd" });
  })();

  return stream;
}

/**
 * Continue an existing session with a new prompt.
 * Provides the prior message history so the model has full context.
 */
export function agentLoopContinue(
  priorMessages: AgentMessage[],
  prompt: string,
  config: AgentLoopConfig,
): EventStream<AgentSessionEvent, AgentMessage[]> {
  const configWithHistory: AgentLoopConfig = config;

  const {
    model,
    systemPrompt,
    tools,
    streamFn,
    onEvent,
    signal,
    maxTurns = 50,
    onToolCall,
    onToolResult,
  } = configWithHistory;

  const stream = new EventStream<AgentSessionEvent, AgentMessage[]>(
    (e) => e.type === "sessionEnd",
    () => messages,
  );

  const messages: AgentMessage[] = [...priorMessages];

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
          emit({ type: "error", error: { message: "Run was aborted." } });
          break;
        }

        if (turnCount >= maxTurns) {
          emit({
            type: "error",
            error: { message: `Maximum turns reached (${maxTurns}).` },
          });
          break;
        }

        turnCount++;
        const turnId = randomUUID();

        const assistantMessage = await streamOneTurn(
          { model, systemPrompt, messages: [...messages], tools, signal },
          streamFn,
          turnId,
          emit,
        );

        emit({ type: "modelStreamEnd", turn: assistantMessage });
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
            executeTool(call, tools, onToolCall, onToolResult).then((result) => {
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

        emit({ type: "turnEnd", turnId });
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      emit({ type: "error", error: { message } });
      stream.fail(err);
      return;
    }

    emit({ type: "sessionEnd" });
  })();

  return stream;
}
