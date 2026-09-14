import type {
  AgentMessage,
  AgentSessionEvent,
  AgentTool,
  ApprovalMode,
  CacheIdentity,
  CacheRetention,
  ImagePart,
  Model,
  PermissionRequest,
  ThinkingLevel,
} from "@/agent/src/types/index.js";
import { createSubagentTool } from "@/agent/src/tools/subagent.js";
import { bindToolCwd } from "@console/types";
import { randomUUID } from "node:crypto";
import type { CompactionOptions } from "../compaction/index.js";
import { createSmolSummarizer } from "../compaction/index.js";
import type { CompactionSummaryFn } from "./types.js";
import { hasConfiguredRole, resolveModelRole } from "./role-resolver.js";
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
  /** Optional provider-aware transport resolver for role models. */
  getStreamFnForModel?: (model: Model) => StreamFn;
  /** Optional runtime thinking override for providers that support it. */
  thinkingLevel?: ThinkingLevel;
  /** Security approval mode ("always-ask" | "accept-edits" | "plan-mode" | "full-access"). Default: "always-ask" */
  approvalMode?: ApprovalMode;
  /** Context window auto-compaction configuration, or `false` to disable. */
  compaction?: CompactionOptions | false;
  /** Hook called when a tool call requires user permission. */
  onApproval?: (request: PermissionRequest) => Promise<boolean> | boolean;
  /** Optional smol-backed compaction summary generator. */
  summarizeCompaction?: CompactionSummaryFn;
  /** Called for every event emitted during a run. */
  onEvent?: (event: AgentSessionEvent) => void;
  /**
   * Provider-neutral cache retention hint. Defaults to "short" so providers
   * that auto-cache keep a warm prefix across turns of one conversation.
   * Set to "none" to disable cache controls even on supporting providers.
   */
  cacheRetention?: CacheRetention;
  /**
   * Optional pre-existing stable cache identity. When omitted, one is
   * generated on construction. The identity rotates when `setModel` changes
   * provider or model id.
   */
  cacheIdentity?: CacheIdentity;
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
  private _getStreamFnForModel?: AgentOptions["getStreamFnForModel"];
  private _thinkingLevel?: ThinkingLevel;
  private _approvalMode: ApprovalMode;
  private _compaction?: CompactionOptions;
  private _onApproval?: AgentOptions["onApproval"];
  private _onEvent?: (event: AgentSessionEvent) => void;
  private summarizeCompaction?: CompactionSummaryFn;
  private _cacheRetention: CacheRetention;
  private _cacheIdentity: CacheIdentity;

  private _messages: AgentMessage[] = [];
  private _abortController?: AbortController;
  private _running = false;

  constructor(options: AgentOptions) {
    this._model = options.model;
    this._tools = options.tools;
    this._systemPrompt = options.systemPrompt ?? "";
    this._streamFn = options.streamFn;
    this._getStreamFnForModel = options.getStreamFnForModel;
    this._thinkingLevel = options.thinkingLevel;
    this._approvalMode = options.approvalMode ?? "always-ask";
    this._onApproval = options.onApproval;
    this._onEvent = options.onEvent;
    this._cacheRetention = options.cacheRetention ?? "short";
    // Stable per-conversation identity. Cached prefixes are not portable
    // across providers or models, so we bind the identity to (provider, model)
    // at construction time. setModel() rotates it when either changes.
    this._cacheIdentity =
      options.cacheIdentity ?? {
        conversationId: randomUUID(),
        provider: options.model.provider,
        modelId: options.model.id,
      };
    this.summarizeCompaction = options.summarizeCompaction;

    if (options.compaction === false) {
      this._compaction = undefined;
    } else {
      this._compaction = {
        enabled: true,
        maxThresholdRatio: 0.85,
        keepRecentTokens: 40_000,
        minimumRecentTurns: 3,
        maxToolResultChars: 8_000,
        summaryStrategy: "structural",
        ...(options.compaction ?? {}),
      };
    }

    // Built after _compaction so the default strategy is visible here. Falls
    // back to structural at call time when no smol model is configured.
    if (!this.summarizeCompaction && this._compaction?.summaryStrategy === "llm") {
      const getFallbackModel = () => this._model;
      const getStreamFn = (model: Model) => this._getStreamFnForModel?.(model) ?? this._streamFn;
      this.summarizeCompaction = createSmolSummarizer({ getFallbackModel, getStreamFn });
    }
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

  /** Current cache retention hint. */
  get cacheRetention(): CacheRetention {
    return this._cacheRetention;
  }

  /**
   * Stable cache identity for this Agent. The conversation id is reused
   * across turns and safe retries; it rotates when the provider or model
   * changes (see setModel).
   */
  get cacheIdentity(): CacheIdentity {
    return this._cacheIdentity;
  }


  // -------------------------------------------------------------------------
  // Configuration setters (can be changed between runs)
  // -------------------------------------------------------------------------

  setModel(model: Model): void {
    // Cached prefixes are not portable across providers or models. Rotating
    // the conversation id here ensures providers treat this as a fresh cache
    // namespace rather than attempting (and failing) to reuse a stale one.
    if (model.provider !== this._model.provider || model.id !== this._model.id) {
      this._cacheIdentity = {
        conversationId: randomUUID(),
        provider: model.provider,
        modelId: model.id,
      };
    }
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

  setCacheRetention(retention: CacheRetention): void {
    this._cacheRetention = retention;
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
  run(
    prompt: string,
    signal?: AbortSignal,
    attachments?: ImagePart[],
  ): EventStream<AgentSessionEvent, AgentMessage[]> {
    if (this._running) {
      throw new AgentBusyError();
    }

    this._running = true;
    this._abortController = new AbortController();

    // If the caller passes their own signal, link it to ours. The signal may
    // already be aborted by the time run() is called (e.g. a steer request
    // landing during the async setup before this call) — addEventListener
    // alone would silently miss that, so check the current state too.
    if (signal) {
      if (signal.aborted) {
        this._abortController.abort();
      } else {
        signal.addEventListener("abort", () => this._abortController?.abort());
      }
    }

    let eventEmitter: ((event: AgentSessionEvent) => void) | undefined;

    const tools = this._tools.map((tool) =>
      tool.name === "subagent"
        ? createSubagentTool({
            model: this._model,
            streamFn: this._streamFn,
            tools: this._tools,
            systemPrompt: this._systemPrompt,
            approvalMode: "full-access",
            onApproval: this._onApproval,
            onEvent: (event) => eventEmitter?.(event),
          })
        : tool,
    );

    const config: AgentLoopConfig = {
      model: this._model,
      systemPrompt: this._systemPrompt,
      tools,
      streamFn: this._streamFn,
      thinkingLevel: this._thinkingLevel,
      approvalMode: this._approvalMode,
      onApproval: this._onApproval,
      compaction: this._compaction,
      summarizeCompaction: this.summarizeCompaction,
      signal: this._abortController.signal,
      onEvent: this._onEvent,
      cacheRetention: this._cacheRetention,
      cacheIdentity: this._cacheIdentity,
    };

    // First run: use agentLoop (adds the prompt as a UserMessage internally)
    // Continue runs: use agentLoopContinue (injects prior history)
    const eventStream =
      this._messages.length === 0
        ? agentLoop(prompt, config, attachments)
        : agentLoopContinue(this._messages, prompt, config, attachments);

    eventEmitter = (event) => {
      this._onEvent?.(event);
      if (event.type === "compaction" && event.compactedMessages) {
        this._messages = [...event.compactedMessages];
      }
      eventStream.push(event);
    };

    // When the run finishes, collect new messages and mark as idle
    eventStream.result().then(
      (finalMessages) => {
        this._messages = [...finalMessages];
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
