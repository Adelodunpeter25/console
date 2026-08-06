/**
 * Agent Run Execution Service.
 * Manages active run controllers, system prompt building, and agent turn execution with real-time turn persistence.
 */
import { Agent } from "../../../agent/src/service/agent.js";
import { allTools } from "../../../agent/src/tools/index.js";
import { createAskTool } from "../../../agent/src/tools/ask.js";
import { createTodoTool, type TodoItem } from "../../../agent/src/tools/todo.js";
import { SqliteSessionStorage } from "../../../agent/src/session/storage.js";
import { buildSystemPrompt } from "../../../agent/src/systemprompt/builder.js";
import { findModelInProvider, getProvider } from "../../../agent/src/commands/provider-registry.js";
import type {
  AgentSessionEvent,
  ApprovalMode,
  Model,
  UserMessage,
} from "../../../agent/src/types/index.js";
import type { RunPromptDto } from "../types/index.js";
import { expandPromptRefs } from "./assist.service.js";
import { randomUUID } from "node:crypto";
import { repairToolCallHistory } from "../../../agent/src/utils/tool-history.js";

export class RunService {
  private sessionStorage = new SqliteSessionStorage();
  private static activeRuns = new Map<string, AbortController>();
  private todoLists = new Map<string, TodoItem[]>();

  public static isRunActive(sessionId: string): boolean {
    return RunService.activeRuns.has(sessionId);
  }
  /** How long a pending question or permission decision may sit unresolved
      before it is rejected. Prevents the agent loop from hanging forever if
      the client disconnects or never answers. */
  private static readonly DECISION_TIMEOUT_MS = 10 * 60 * 1000;
  /** Pending question answers keyed by requestId. */
  private pendingQuestions = new Map<
    string,
    {
      sessionId: string;
      resolve: (answer: string | string[]) => void;
      reject: (err: unknown) => void;
    }
  >();
  /** Pending permission approvals keyed by requestId. */
  private pendingApprovals = new Map<
    string,
    { sessionId: string; resolve: (allow: boolean) => void; reject: (err: unknown) => void }
  >();

  /**
   * Execute an agent run, streaming lifecycle events to onEvent callback.
   * Persists completed turns incrementally so data is never lost during network drops or crashes.
   */
  async runAgentStream(
    sessionId: string,
    dto: RunPromptDto,
    onEvent: (event: AgentSessionEvent) => Promise<void> | void,
  ): Promise<void> {
    if (RunService.activeRuns.has(sessionId)) {
      throw new Error(`Session '${sessionId}' already has an active run.`);
    }

    const abortController = new AbortController();
    RunService.activeRuns.set(sessionId, abortController);
    try {
      await this.runAgentStreamInternal(sessionId, dto, onEvent, abortController);
    } finally {
      RunService.activeRuns.delete(sessionId);
    }
  }

  private async runAgentStreamInternal(
    sessionId: string,
    dto: RunPromptDto,
    onEvent: (event: AgentSessionEvent) => Promise<void> | void,
    abortController: AbortController,
  ): Promise<void> {
    // Resolve @path file mentions against the session's working directory so
    // the agent can read referenced files directly. Applied once up-front so
    // the expanded prompt feeds both the auto-title and the agent run.
    let session = this.sessionStorage.loadSession(sessionId);
    const prompt = expandPromptRefs(dto.prompt.trim(), session?.header.cwd ?? process.cwd());
    if (!session) {
      const cwd = process.cwd();
      const autoTitle = prompt.length > 35 ? `${prompt.slice(0, 35)}...` : prompt;
      const header = this.sessionStorage.createSession({
        id: sessionId,
        title: autoTitle,
        cwd,
        modelId: dto.modelId || "gemini-2.5-pro",
        provider: dto.provider || "antigravity",
      });
      session = { header, messages: [] };
    } else {
      const currentTitle = session.header.title;
      const isGenericTitle =
        !currentTitle ||
        currentTitle === "New Session" ||
        currentTitle === "New mobile session" ||
        currentTitle === "New Chat" ||
        currentTitle === "Untitled";

      if (isGenericTitle || session.messages.length === 0) {
        const autoTitle = prompt.length > 35 ? `${prompt.slice(0, 35)}...` : prompt;
        this.sessionStorage.updateTitle(sessionId, autoTitle);
        session.header.title = autoTitle;
      }
    }

    const repairedHistory = repairToolCallHistory(session.messages);
    if (repairedHistory.repaired) {
      session.messages = repairedHistory.messages;
      this.sessionStorage.replaceMessages(sessionId, session.messages);
    }

    const provider = dto.provider || session.header.provider || "antigravity";
    const modelId = dto.modelId || session.header.modelId || "gemini-2.5-pro";
    const catalogModel = findModelInProvider(provider, modelId);
    if (dto.attachments && dto.attachments.length > 0 && catalogModel?.supportsImages === false) {
      throw new Error(`The selected model '${modelId}' does not support image attachments.`);
    }
    // Use the approvalMode from the request; fall back to the persisted session value,
    // then to "always-ask" as the safe default. Never silently run without a mode.
    const approvalMode = (dto.approvalMode ||
      session.header.approvalMode ||
      "always-ask") as ApprovalMode;

    // Persist the chosen approvalMode to the DB so it survives reloads.
    this.sessionStorage.updateApprovalMode(sessionId, approvalMode);

    const model: Model = {
      id: modelId,
      provider: provider as Model["provider"],
      contextWindow: 128_000,
      ...(typeof catalogModel?.supportsImages === "boolean"
        ? { supportsImages: catalogModel.supportsImages }
        : {}),
    };

    const providerEntry = getProvider(provider);
    const streamFn = providerEntry?.getStreamFn();
    if (!streamFn) {
      throw new Error(`Unknown provider '${provider}'.`);
    }

    const { systemPrompt } = await buildSystemPrompt({
      cwd: session.header.cwd,
      model: modelId,
      approvalMode,
    });

    // Wire the ask-question handler so the ask tool pauses for user input
    // instead of auto-selecting the first option. The handler emits the
    // askQuestion event through onEvent and waits for answerQuestion().
    const askTool = createAskTool((request) => {
      return new Promise<string | string[]>((resolve, reject) => {
        this.pendingQuestions.set(request.requestId, { sessionId, resolve, reject });
        onEvent({ type: "askQuestion", request });
        this.startDecisionTimeout(request.requestId, sessionId, "question");
      });
    });

    const sessionTodo = createTodoTool(this.todoLists.get(sessionId) ?? [], (items, action) => {
      this.todoLists.set(sessionId, items);
      return onEvent({ type: "todoUpdate", items, action });
    });
    const tools = allTools.map((tool) => {
      if (tool.name === "ask") return askTool;
      if (tool.name === "todo") return sessionTodo.tool;
      return tool;
    });

    const agent = new Agent({
      model,
      tools: tools as any,
      systemPrompt,
      streamFn,
      approvalMode,
      onApproval: (req) => {
        // executeTool already emitted the permissionRequest event;
        // we just need to wait for the user's decision.
        return new Promise<boolean>((resolve, reject) => {
          this.pendingApprovals.set(req.requestId, { sessionId, resolve, reject });
          this.startDecisionTimeout(req.requestId, sessionId, "permission");
        });
      },
    });

    // Persist the user message immediately so it survives crashes, errors, and
    // session switches even if the run never completes. The agent also appends
    // this prompt to its internal history; we skip the duplicate at the end.
    const userMessage: UserMessage = {
      role: "user",
      content: prompt,
      ...(dto.attachments && dto.attachments.length > 0 ? { attachments: dto.attachments } : {}),
    };
    this.sessionStorage.appendMessage(sessionId, userMessage);

    agent.loadHistory(session.messages);

    this.sessionStorage.updateSessionStatus(sessionId, "working");
    const runPersistenceId = randomUUID();
    let toolBatchNumber = 0;
    let toolResultsPersistenceId: string | null = null;

    try {
      const eventStream = agent.run(prompt, abortController.signal, dto.attachments);

      // Track run failures reported by the agent loop (stream errors, max
      // turns, provider failures) so they can be persisted as error messages.
      let runError: string | null = null;

      for await (const event of eventStream) {
        // Persist completed units as events arrive. This prevents a crash or
        // disconnect after a tool finishes from losing the result entirely.
        if (event.type === "toolExecutionStart") {
          toolResultsPersistenceId = `tool-results:${runPersistenceId}:${toolBatchNumber++}`;
        }
        if (event.type === "modelStreamEnd" && event.turn) {
          this.sessionStorage.appendMessage(sessionId, event.turn);
        }
        if (event.type === "toolExecutionResult" && toolResultsPersistenceId) {
          this.sessionStorage.upsertToolResult(sessionId, toolResultsPersistenceId, event.result);
        }
        if (event.type === "toolExecutionEnd") {
          for (const result of event.results) {
            if (!toolResultsPersistenceId) continue;
            this.sessionStorage.upsertToolResult(sessionId, toolResultsPersistenceId, result);
          }
          toolResultsPersistenceId = null;
        }

        // Mark needs_attention when the agent asks a question or requests permission
        if (event.type === "askQuestion" || event.type === "permissionRequest") {
          if (event.type === "permissionRequest") {
            console.info("[permission] server emitted request", {
              sessionId,
              requestId: event.request.requestId,
              toolName: event.request.toolName,
              tier: event.request.tier,
              requiresUpgrade: event.request.requiresUpgrade,
            });
          }
          this.sessionStorage.updateSessionStatus(sessionId, "needs_attention");
        }

        await onEvent(event);

        if (event.type === "error") {
          runError = event.error?.message ?? "Unknown agent error";
        }
      }

      await eventStream.result();

      // Persist run failures as an error bubble so they survive reloads.
      if (runError) {
        this.sessionStorage.appendMessage(sessionId, {
          role: "assistant",
          content: [{ type: "text", text: `Error: ${runError}` }],
        });
      }
      this.sessionStorage.updateSessionStatus(sessionId, runError ? "needs_attention" : "done");
    } catch (err) {
      // User-initiated abort is normal control flow — don't mark as needs_attention.
      const isAbort =
        abortController.signal.aborted ||
        (err instanceof Error && err.name === "AbortError") ||
        (err instanceof Error && err.message === "This operation was aborted.");
      if (isAbort) {
        this.sessionStorage.updateSessionStatus(sessionId, "done");
      } else {
        this.sessionStorage.updateSessionStatus(sessionId, "needs_attention");
        // Persist the failure so the error survives session switches.
        const errorMsg = err instanceof Error ? err.message : String(err);
        this.sessionStorage.appendMessage(sessionId, {
          role: "assistant",
          content: [{ type: "text", text: `Error: ${errorMsg}` }],
        });
        throw err;
      }
    } finally {
      // Reject any unresolved pending questions/approvals so the agent
      // loop doesn't hang forever after the run ends.
      for (const [requestId, pending] of this.pendingQuestions) {
        if (pending.sessionId !== sessionId) continue;
        this.pendingQuestions.delete(requestId);
        pending.reject(new Error("Run ended"));
      }
      for (const [requestId, pending] of this.pendingApprovals) {
        if (pending.sessionId !== sessionId) continue;
        this.pendingApprovals.delete(requestId);
        pending.reject(new Error("Run ended"));
      }
    }
  }

  /**
   * Answer a pending question from the ask tool.
   */
  answerQuestion(sessionId: string, requestId: string, answer: string | string[]): boolean {
    const pending = this.pendingQuestions.get(requestId);
    if (!pending || pending.sessionId !== sessionId) return false;
    this.pendingQuestions.delete(requestId);
    pending.resolve(answer);
    return true;
  }

  /**
   * Approve or deny a pending tool permission request.
   */
  approvePermission(sessionId: string, requestId: string, allow: boolean): boolean {
    const pending = this.pendingApprovals.get(requestId);
    if (!pending || pending.sessionId !== sessionId) return false;
    this.pendingApprovals.delete(requestId);
    pending.resolve(allow);
    return true;
  }

  /**
   * Arm a timeout for a pending question or permission decision. If the client
   * never answers, the pending promise rejects so the agent loop can surface an
   * error instead of hanging forever.
   */
  private startDecisionTimeout(
    requestId: string,
    sessionId: string,
    kind: "question" | "permission",
  ): void {
    setTimeout(() => {
      const pending =
        kind === "question"
          ? this.pendingQuestions.get(requestId)
          : this.pendingApprovals.get(requestId);
      if (!pending || pending.sessionId !== sessionId) return;
      if (kind === "question") {
        this.pendingQuestions.delete(requestId);
      } else {
        this.pendingApprovals.delete(requestId);
      }
      pending.reject(
        new Error(
          `${kind === "question" ? "Question" : "Permission request"} timed out waiting for a decision.`,
        ),
      );
    }, RunService.DECISION_TIMEOUT_MS).unref?.();
  }

  /**
   * Abort an active run for a session.
   */
  abortRun(sessionId: string): boolean {
    const controller = RunService.activeRuns.get(sessionId);
    if (!controller) return false;

    controller.abort();
    for (const [requestId, pending] of this.pendingQuestions) {
      if (pending.sessionId !== sessionId) continue;
      this.pendingQuestions.delete(requestId);
      pending.reject(new Error("Run aborted while waiting for a question."));
    }
    for (const [requestId, pending] of this.pendingApprovals) {
      if (pending.sessionId !== sessionId) continue;
      this.pendingApprovals.delete(requestId);
      pending.reject(new Error("Run aborted while waiting for permission."));
    }
    return true;
  }
}
