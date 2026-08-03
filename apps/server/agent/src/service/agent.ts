import type {
  AgentMessage,
  AgentSessionEvent,
  AgentTool,
  ApprovalMode,
  Model,
  PermissionRequest,
} from "../types/index.js";
import { createTaskTool } from "../tools/task.js";
import { agentLoop, agentLoopContinue, type AgentLoopConfig, type StreamFn } from "./agent-loop.js";
import type { EventStream } from "./event-stream.js";

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

export class AgentBusyError extends Error {
  constructor(
    message = "Agent is already running. Call abort() first or wait for the current run to finish.",
  ) {
    super(message);
    this.name = "AgentBusyError";
  }
}

// ---------------------------------------------------------------------------
// Agent options
// ---------------------------------------------------------------------------

export interface AgentOptions {
  model: Model;
  tools: AgentTool[];
  systemPrompt?: string;
  streamFn: StreamFn;
  /** Security approval mode ("always-ask" | "accept-edits" | "plan-mode" | "full-access"). Default: "always-ask" */
  approvalMode?: ApprovalMode;
  /** Hook called when a tool call requires user permission. */
  onApproval?: (request: PermissionRequest) => Promise<boolean> | boolean;
  /** Max agentic turns per run. Default: 50 */
  maxTurns?: number;
  /** Called for every event emitted during a run. */
  onEvent?: (event: AgentSessionEvent) => void;
}

// ---------------------------------------------------------------------------
// Agent class
// ---------------------------------------------------------------------------

/**
 * Stateful agent that maintains conversation history across runs.
 */
export class Agent {
  private _model: Model;
  private _tools: AgentTool[];
  private _systemPrompt: string;
  private _streamFn: StreamFn;
  private _approvalMode: ApprovalMode;
  private _onApproval?: AgentOptions["onApproval"];
  private _maxTurns: number;
  private _onEvent?: (event: AgentSessionEvent) => void;

  private _messages: AgentMessage[] = [];
  private _abortController?: AbortController;
  private _running = false;

  constructor(options: AgentOptions) {
    this._model = options.model;
    this._tools = options.tools;
    this._systemPrompt = options.systemPrompt ?? "";
    this._streamFn = options.streamFn;
    this._approvalMode = options.approvalMode ?? "always-ask";
    this._onApproval = options.onApproval;
    this._maxTurns = options.maxTurns ?? 50;
    this._onEvent = options.onEvent;
  }

  // -------------------------------------------------------------------------
  // Public properties
  // -------------------------------------------------------------------------

  /** Read-only snapshot of the current conversation history. */
  get messages(): readonly AgentMessage[] {
    return this._messages;
  }

  /** Whether a run is currently in progress. */
  get isRunning(): boolean {
    return this._running;
  }

  get model(): Model {
    return this._model;
  }

  get tools(): AgentTool[] {
    return this._tools;
  }

  get systemPrompt(): string {
    return this._systemPrompt;
  }

  get approvalMode(): ApprovalMode {
    return this._approvalMode;
  }

  // -------------------------------------------------------------------------
  // Configuration setters (can be changed between runs)
  // -------------------------------------------------------------------------

  setModel(model: Model): void {
    this._model = model;
  }

  setTools(tools: AgentTool[]): void {
    this._tools = tools;
  }

  setSystemPrompt(prompt: string): void {
    this._systemPrompt = prompt;
  }

  setStreamFn(streamFn: StreamFn): void {
    this._streamFn = streamFn;
  }

  setApprovalMode(mode: ApprovalMode): void {
    this._approvalMode = mode;
  }

  setOnApproval(onApproval?: AgentOptions["onApproval"]): void {
    this._onApproval = onApproval;
  }

  // -------------------------------------------------------------------------
  // Run management
  // -------------------------------------------------------------------------

  /**
   * Run the agent with a new user prompt.
   * Throws AgentBusyError if a run is already in progress.
   * Returns an EventStream you can subscribe to with for-await-of.
   * New messages are appended to this.messages when the run completes.
   */
  run(prompt: string, signal?: AbortSignal): EventStream<AgentSessionEvent, AgentMessage[]> {
    if (this._running) {
      throw new AgentBusyError();
    }

    this._running = true;
    this._abortController = new AbortController();

    // If the caller passes their own signal, link it to ours
    if (signal) {
      signal.addEventListener("abort", () => this._abortController?.abort());
    }

    const tools = this._tools.map((tool) =>
      tool.name === "task"
        ? createTaskTool({
            model: this._model,
            streamFn: this._streamFn,
            tools: this._tools,
            systemPrompt: this._systemPrompt,
          })
        : tool,
    );

    const config: AgentLoopConfig = {
      model: this._model,
      systemPrompt: this._systemPrompt,
      tools,
      streamFn: this._streamFn,
      approvalMode: this._approvalMode,
      onApproval: this._onApproval,
      signal: this._abortController.signal,
      maxTurns: this._maxTurns,
      onEvent: this._onEvent,
    };

    // First run: use agentLoop (adds the prompt as a UserMessage internally)
    // Continue runs: use agentLoopContinue (injects prior history)
    const eventStream =
      this._messages.length === 0
        ? agentLoop(prompt, config)
        : agentLoopContinue(this._messages, prompt, config);

    // When the run finishes, collect new messages and mark as idle
    eventStream.result().then(
      (newMessages) => {
        // Append only the NEW messages (those not already in history)
        const newOnly = newMessages.slice(this._messages.length);
        this._messages.push(...newOnly);
        this._running = false;
        this._abortController = undefined;
      },
      () => {
        // On error, still mark as idle
        this._running = false;
        this._abortController = undefined;
      },
    );

    return eventStream;
  }

  /**
   * Abort the current run. No-op if no run is in progress.
   */
  abort(): void {
    this._abortController?.abort();
  }

  /**
   * Append messages to the agent's history.
   * Use this to restore a persisted session.
   */
  loadHistory(messages: AgentMessage[]): void {
    if (this._running) {
      throw new AgentBusyError("Cannot load history while a run is in progress.");
    }
    this._messages.push(...messages);
  }

  /**
   * Replace the agent's history entirely.
   */
  setHistory(messages: AgentMessage[]): void {
    if (this._running) {
      throw new AgentBusyError("Cannot set history while a run is in progress.");
    }
    this._messages = [...messages];
  }

  /**
   * Clear the conversation history.
   */
  clearHistory(): void {
    if (this._running) {
      throw new AgentBusyError("Cannot clear history while a run is in progress.");
    }
    this._messages = [];
  }
}
