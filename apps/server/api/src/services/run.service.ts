/**
 * Agent Run Execution Service.
 * Manages active run controllers, system prompt building, and agent turn execution with real-time turn persistence.
 */
import { Agent } from "@/agent/src/service/agent.js";
import { allTools } from "@/agent/src/tools/index.js";
import { createAskManyTool, createAskTool } from "@/agent/src/tools/ask.js";
import { createTodoTool, type TodoItem } from "@/agent/src/tools/todo.js";
import { SqliteSessionStorage } from "@/agent/src/session/storage.js";
import { buildSystemPrompt } from "@/agent/src/systemprompt/builder.js";
import { findModelInProvider, getProvider } from "@/agent/src/commands/provider-registry.js";
import type {
  AgentSessionEvent,
  AgentTool,
  ApprovalMode,
  AskQuestionRequest,
  ImagePart,
  Model,
  UserMessage,
} from "@console/types";
import { bindToolCwd } from "@console/types";
import type { RunPromptDto } from "@/api/src/types/index.js";
import { expandPromptRefs } from "./assist.service.js";
import { randomUUID } from "node:crypto";
import {
  attentionNotification,
  doneNotification,
  isAttentionEvent,
  isDoneEvent,
} from "./notify-agent-event.js";
import { notificationService } from "./notification.service.js";
import type { RunStreamSubscriber } from "@console/types";
import { RunEventHub } from "@console/types";
import { extractErrorMessage } from "@/agent/src/utils/error.js";

export class RunService {
  private sessionStorage = new SqliteSessionStorage();
  private static activeRuns = new Map<string, AbortController>();
  private todoLists = new Map<string, TodoItem[]>();
  /** Live run fan-out hubs keyed by sessionId (one per in-flight run). */
  private hubs = new Map<string, RunEventHub>();

  private ensureHub(sessionId: string): RunEventHub {
    let hub = this.hubs.get(sessionId);
    if (!hub) {
      hub = new RunEventHub();
      this.hubs.set(sessionId, hub);
    }
    return hub;
  }

  public static isRunActive(sessionId: string): boolean {
    return RunService.activeRuns.has(sessionId);
  }

  /**
   * Attach a subscriber to the in-flight run for a session.
   * Returns false when no run is active (caller should fall back to loading
   * persisted messages). When `since` is given, buffered events with seq >
   * since are replayed ahead of live delivery — snapshot + registration happen
   * in one synchronous section, so nothing is missed or duplicated.
   */
  subscribeToActiveRun(
    sessionId: string,
    subscriber: RunStreamSubscriber,
    since?: number,
  ): boolean {
    const hub = this.hubs.get(sessionId);
    if (!hub) return false;
    hub.subscribe(subscriber, since);
    return true;
  }

  /** Detach a previously attached subscriber (e.g. its SSE stream closed). */
  unsubscribeFromActiveRun(sessionId: string, subscriberId: string): void {
    this.hubs.get(sessionId)?.unsubscribe(subscriberId);
  }

  /** Resolves once any active run for the session settles (immediately if none). */
  waitForRunSettle(sessionId: string): Promise<void> {
    return this.hubs.get(sessionId)?.settled ?? Promise.resolve();
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
   *
   * The initiating client's onEvent is registered as the primary subscriber of a
   * per-run event hub; other surfaces (mobile re-attach, desktop) can join the
   * same run live via subscribeToActiveRun(). The legacy POST /run pipe keeps
   * its exact wire behavior — synthetic frames (done/aborted/streamReset) are
   * only delivered to subscribers that opt into them.
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

    const hub = this.ensureHub(sessionId);
    const primarySubscriber: RunStreamSubscriber = {
      id: randomUUID(),
      deliver: (_seq, event) => onEvent(event),
      // No ping/extendedFrames: preserve the exact legacy POST /run wire.
    };
    hub.subscribe(primarySubscriber);

    try {
      await this.runAgentStreamInternal(sessionId, dto, hub, abortController);
    } finally {
      RunService.activeRuns.delete(sessionId);
      hub.destroy();
      this.hubs.delete(sessionId);
    }
  }

  private async runAgentStreamInternal(
    sessionId: string,
    dto: RunPromptDto,
    hub: RunEventHub,
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

    // Recover histories left dirty by a crashed process before loading them
    // into the agent. Settled runs mark the session repaired below, so this
    // path is only a recovery check for interrupted runs.
    if (session.messages.length > 0 && this.sessionStorage.repairSession(sessionId)) {
      session = this.sessionStorage.loadSession(sessionId) ?? session;
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

    // Persist the chosen model/provider and approval mode so the session
    // configuration survives reloads and remains consistent with this run.
    this.sessionStorage.updateModel(sessionId, modelId, provider);
    this.sessionStorage.updateApprovalMode(sessionId, approvalMode);

    const model: Model = {
      id: modelId,
      provider: provider as Model["provider"],
      contextWindow: catalogModel?.contextWindow ?? 128_000,
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
    // instead of auto-selecting the first option. The handler broadcasts the
    // askQuestion event through the run hub and waits for answerQuestion().
    const askHandler = (request: AskQuestionRequest) => {
      return new Promise<string | string[]>((resolve, reject) => {
        this.pendingQuestions.set(request.requestId, { sessionId, resolve, reject });
        hub.broadcast({ type: "askQuestion", request });
        this.startDecisionTimeout(request.requestId, sessionId, "question");
      });
    };
    // Bind every tool to the session's working directory so tools like glob and
    // bash operate on the project the user selected instead of falling back to
    // the server's process.cwd(). Subagent is replaced by Agent.run() with a
    // contextual version that carries the same bound tools into the child loop.
    const tools = allTools.map((tool) => bindToolCwd(tool as AgentTool, session.header.cwd));

    const askTool = createAskTool(askHandler);
    const askManyTool = createAskManyTool(askHandler);

    const sessionTodo = createTodoTool(this.todoLists.get(sessionId) ?? [], (items, action) => {
      this.todoLists.set(sessionId, items);
      hub.broadcast({ type: "todoUpdate", items, action });
    });

    const boundTools = tools.map((tool) => {
      if (tool.name === "ask") return askTool;
      if (tool.name === "askMany") return askManyTool;
      if (tool.name === "todo") return sessionTodo.tool;
      return tool;
    });

    const agent = new Agent({
      model,
      tools: boundTools as any,
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
    // DTO attachments arrive untagged; tag them as image parts for the loop
    // and provider conversion layers.
    const attachments: ImagePart[] | undefined = dto.attachments?.map((a) => ({
      type: "image" as const,
      data: a.data,
      mimeType: a.mimeType,
    }));
    const userMessage: UserMessage = {
      role: "user",
      content: prompt,
      ...(attachments && attachments.length > 0 ? { attachments } : {}),
    };
    this.sessionStorage.appendMessage(sessionId, userMessage);

    agent.loadHistory(session.messages);

    // A new run can create an interrupted tool turn, so its final persistence
    // pass must inspect the history even when the previous run was clean.
    this.sessionStorage.markSessionNeedsRepair(sessionId);
    this.sessionStorage.updateSessionStatus(sessionId, "working");
    const runPersistenceId = randomUUID();
    let toolBatchNumber = 0;
    let toolResultsPersistenceId: string | null = null;

    try {
      const eventStream = agent.run(prompt, abortController.signal, attachments);

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
          this.sessionStorage.updateSessionStatus(sessionId, "needs_attention");
        }

        // Emit native notifications for attention-worthy and completion events.
        if (isAttentionEvent(event)) {
          notificationService.push(attentionNotification(sessionId, event));
        } else if (isDoneEvent(event) && !runError) {
          notificationService.push(doneNotification(sessionId));
        }

        hub.broadcast(event);

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
        const errorMsg = extractErrorMessage(err);
        this.sessionStorage.appendMessage(sessionId, {
          role: "assistant",
          content: [{ type: "text", text: `Error: ${errorMsg}` }],
        });
        throw err;
      }
    } finally {
      // Repair once after the run settles, rather than on every session read.
      this.sessionStorage.repairSession(sessionId);

      // Terminal signal for re-attach subscribers (extendedFrames only — the
      // legacy POST /run pipe just closes, as before).
      hub.broadcast(
        abortController.signal.aborted
          ? { type: "aborted", reason: "Run was aborted." }
          : { type: "done" },
      );

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
    const timer = setTimeout(() => {
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
    }, RunService.DECISION_TIMEOUT_MS);
    // Bun's timer is a number id; Node's is a Timeout object — only Node can unref.
    if (typeof timer === "object") timer.unref?.();
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
