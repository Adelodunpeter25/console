/**
 * Agent Run Execution Service.
 * Coordinates active run controllers, streaming lifecycle events, and turn persistence.
 */
import { Agent } from "@/agent/src/service/agent.js";
import { type TodoItem } from "@/agent/src/tools/todo.js";
import { getSharedSessionStorage } from "@/agent/src/session/storage.js";
import { buildSystemPrompt } from "@/agent/src/systemprompt/builder.js";
import {
  DEFAULT_FALLBACK_MODEL,
  findModelInProvider,
  getProvider,
} from "@/agent/src/commands/provider-registry.js";
import type {
  AgentSessionEvent,
  ApprovalMode,
  ImagePart,
  QueuedPrompt,
  UserMessage,
} from "@console/types";
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
import type { RunStreamSubscriber } from "@console/utils";
import { RunEventHub } from "@console/utils";
import { extractErrorMessage } from "@/agent/src/utils/error.js";
import { DecisionManager } from "./run/run-decisions.js";
import { assembleAgentTools, buildRunModel } from "./run/run-tools.js";
import { extractAndRecordFileChange } from "./run/run-file-changes.js";
import { generateSessionTitle, isGenericSessionTitle } from "@/agent/src/service/session-title.js";
import { resolveModelRole } from "@/agent/src/service/role-resolver.js";

export class RunService {
  private sessionStorage = getSharedSessionStorage();
  private static activeRuns = new Map<string, AbortController>();
  private static pendingNextTurn = new Map<string, RunPromptDto>();
  private todoLists = new Map<string, TodoItem[]>();
  private hubs = new Map<string, RunEventHub>();
  private decisions = new DecisionManager();

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

  unsubscribeFromActiveRun(sessionId: string, subscriberId: string): void {
    this.hubs.get(sessionId)?.unsubscribe(subscriberId);
  }

  waitForRunSettle(sessionId: string): Promise<void> {
    return this.hubs.get(sessionId)?.settled ?? Promise.resolve();
  }

  /**
   * Stage (or replace) the prompt that runs automatically once the session's
   * active turn settles. Shared by both `POST /queue` (auto-pop) and
   * `POST /steer` (abort now, drain sooner) — see the turn loop in
   * `runAgentStream` for the single place staged prompts are taken and run.
   * Returns null when the session does not exist (nothing is staged).
   */
  queuePrompt(sessionId: string, dto: RunPromptDto): QueuedPrompt | null {
    if (this.sessionStorage.loadSession(sessionId) === null) return null;
    const queuedPrompt: QueuedPrompt = {
      id: randomUUID(),
      sessionId,
      prompt: dto.prompt,
      attachments: dto.attachments,
      modelId: dto.modelId,
      provider: dto.provider,
      approvalMode: dto.approvalMode,
      createdAt: new Date().toISOString(),
    };
    RunService.pendingNextTurn.set(sessionId, dto);
    this.sessionStorage.saveQueuedPrompt(sessionId, queuedPrompt);
    this.hubs.get(sessionId)?.broadcast({ type: "queueUpdated", queuedPrompt });
    return queuedPrompt;
  }

  getQueuedPrompt(sessionId: string): QueuedPrompt | null {
    return this.sessionStorage.getQueuedPrompt(sessionId);
  }

  /**
   * Edit the staged prompt in place (keeps its id so clients can tell an edit
   * apart from a delete+queue). Returns null when no prompt is staged.
   */
  editQueuedPrompt(sessionId: string, dto: RunPromptDto): QueuedPrompt | null {
    const existing = this.sessionStorage.getQueuedPrompt(sessionId);
    if (!existing) return null;
    const updated: QueuedPrompt = {
      ...existing,
      prompt: dto.prompt,
      attachments: dto.attachments,
      modelId: dto.modelId,
      provider: dto.provider,
      approvalMode: dto.approvalMode,
    };
    RunService.pendingNextTurn.set(sessionId, dto);
    this.sessionStorage.saveQueuedPrompt(sessionId, updated);
    this.hubs.get(sessionId)?.broadcast({ type: "queueUpdated", queuedPrompt: updated });
    return updated;
  }

  /** Discards the queued prompt without affecting an in-flight run. */
  clearQueuedPrompt(sessionId: string): boolean {
    const had = RunService.pendingNextTurn.has(sessionId) || this.getQueuedPrompt(sessionId) !== null;
    RunService.pendingNextTurn.delete(sessionId);
    this.sessionStorage.clearQueuedPrompt(sessionId);
    this.hubs.get(sessionId)?.broadcast({ type: "queueUpdated", queuedPrompt: null });
    return had;
  }

  /**
   * Take the staged next-turn prompt, if any. Prefers the in-memory entry;
   * falls back to the persisted row so a queue staged before a server restart
   * still drains instead of lingering forever. Clears both stores.
   */
  private takeQueuedDto(sessionId: string): RunPromptDto | undefined {
    const pending = RunService.pendingNextTurn.get(sessionId);
    if (pending) {
      RunService.pendingNextTurn.delete(sessionId);
      this.sessionStorage.clearQueuedPrompt(sessionId);
      return pending;
    }
    const stored = this.sessionStorage.getQueuedPrompt(sessionId);
    if (!stored) return undefined;
    this.sessionStorage.clearQueuedPrompt(sessionId);
    return {
      prompt: stored.prompt,
      attachments: stored.attachments,
      modelId: stored.modelId,
      provider: stored.provider,
      approvalMode: stored.approvalMode,
    };
  }

  /**
   * Halt the active run and arrange for `dto` to start as the next turn as
   * soon as the aborted run settles. Does NOT call `runAgentStream` itself —
   * the drain happens in the same turn loop that auto-pops a queued prompt
   * on normal completion, so a steer request can never race a natural settle
   * into starting two turns.
   */
  steer(sessionId: string, dto: RunPromptDto): boolean {
    if (!RunService.activeRuns.has(sessionId)) return false;
    this.queuePrompt(sessionId, dto);
    this.abortRun(sessionId);
    return true;
  }

  async runAgentStream(
    sessionId: string,
    dto: RunPromptDto,
    onEvent: (event: AgentSessionEvent) => Promise<void> | void,
  ): Promise<void> {
    if (RunService.activeRuns.has(sessionId)) {
      throw new Error(`Session '${sessionId}' already has an active run.`);
    }

    // One hub for the whole chain of turns: chained turns reuse it, so live
    // SSE subscribers keep one connection with gap-free monotonic seq numbers
    // instead of re-attaching to a fresh hub (whose restarted seq would
    // silently drop the next turn's opening events).
    const hub = this.ensureHub(sessionId);
    const primarySubscriber: RunStreamSubscriber = {
      id: randomUUID(),
      deliver: (_seq, event) => onEvent(event),
    };
    hub.subscribe(primarySubscriber);

    try {
      let current: RunPromptDto | undefined = dto;
      let abortController = new AbortController();
      RunService.activeRuns.set(sessionId, abortController);
      while (current) {
        const turnDto = current;
        current = undefined;
        let turnError: unknown;
        try {
          await this.runAgentStreamInternal(sessionId, turnDto, hub, abortController);
        } catch (err) {
          turnError = err;
        }
        // Drain a staged next turn in exactly this section — the same place
        // `activeRuns` is handed over — so a steer request landing at the
        // instant a turn settles can never race a second run into starting,
        // and `isRunActive` never flickers mid-chain (no 409 window for
        // re-attaching clients). Everything here is synchronous. A failed
        // turn holds (not auto-runs, not drops) any staged prompt so the user
        // can steer, edit, or delete it.
        const next = turnError === undefined ? this.takeQueuedDto(sessionId) : undefined;
        if (next) {
          hub.broadcast({ type: "queueUpdated", queuedPrompt: null });
        }
        abortController = new AbortController();
        if (next) RunService.activeRuns.set(sessionId, abortController);
        else RunService.activeRuns.delete(sessionId);
        current = next;
        if (turnError !== undefined) throw turnError;
      }
    } finally {
      hub.unsubscribe(primarySubscriber.id);
      await hub.destroy();
      this.hubs.delete(sessionId);
    }
  }

  private async runAgentStreamInternal(
    sessionId: string,
    dto: RunPromptDto,
    hub: RunEventHub,
    abortController: AbortController,
  ): Promise<void> {
    let session = this.sessionStorage.loadSession(sessionId);
    const prompt = expandPromptRefs(dto.prompt.trim(), session?.header.cwd ?? process.cwd());
    if (!session) {
      const cwd = process.cwd();
      const autoTitle = prompt.length > 35 ? `${prompt.slice(0, 35)}...` : prompt;
      const header = this.sessionStorage.createSession({
        id: sessionId,
        title: autoTitle,
        cwd,
        modelId: dto.modelId || DEFAULT_FALLBACK_MODEL,
        provider: dto.provider || "antigravity",
      });
      session = { header, messages: [] };
    } else {
      const currentTitle = session.header.title?.trim();
      const isGenericTitle =
        !currentTitle ||
        currentTitle === "New Session" ||
        currentTitle === "New mobile session" ||
        currentTitle === "New Chat" ||
        currentTitle === "New chat" ||
        currentTitle === "Untitled";

      if (isGenericTitle) {
        const autoTitle = prompt.length > 35 ? `${prompt.slice(0, 35)}...` : prompt;
        this.sessionStorage.updateTitle(sessionId, autoTitle);
        session.header.title = autoTitle;
      }
    }

    if (session.messages.length > 0 && this.sessionStorage.repairSession(sessionId)) {
      session = this.sessionStorage.loadSession(sessionId) ?? session;
    }

    const provider = dto.provider || session.header.provider || "antigravity";
    const modelId = dto.modelId || session.header.modelId || DEFAULT_FALLBACK_MODEL;
    const catalogModel = findModelInProvider(provider, modelId);
    if (dto.attachments && dto.attachments.length > 0 && catalogModel?.supportsImages === false) {
      throw new Error(`The selected model '${modelId}' does not support image attachments.`);
    }

    const approvalMode = (dto.approvalMode ||
      session.header.approvalMode ||
      "always-ask") as ApprovalMode;

    this.sessionStorage.updateModel(sessionId, modelId, provider);
    this.sessionStorage.updateApprovalMode(sessionId, approvalMode);

    let model = buildRunModel(provider, modelId);
    const providerEntry = getProvider(provider);
    let streamFn = providerEntry?.getStreamFn();
    if (!streamFn) {
      throw new Error(`Unknown provider '${provider}'.`);
    }

    const role = approvalMode === "plan-mode" ? "plan" : "default";
    model = await resolveModelRole(role, model);
    if (model.provider !== provider) {
      const roleProvider = getProvider(model.provider);
      if (roleProvider) streamFn = roleProvider.getStreamFn();
    }

    if (dto.attachments && dto.attachments.length > 0 && model.supportsImages === false) {
      const visionModel = await resolveModelRole("vision", model);
      if (visionModel.supportsImages !== false) {
        model = visionModel;
        const visionProvider = getProvider(model.provider);
        if (visionProvider) streamFn = visionProvider.getStreamFn();
      }
    }

    const { systemPrompt } = await buildSystemPrompt({
      cwd: session.header.cwd,
      model: modelId,
      approvalMode,
    });

    const boundTools = assembleAgentTools({
      cwd: session.header.cwd,
      initialTodos: this.todoLists.get(sessionId) ?? this.sessionStorage.getSessionTodos(sessionId) ?? [],
      askHandler: this.decisions.createAskHandler(sessionId, hub),
      onTodoUpdate: (items, action) => {
        this.todoLists.set(sessionId, items);
        this.sessionStorage.saveSessionTodos(sessionId, items);
        hub.broadcast({ type: "todoUpdate", items, action });
      },
    });

    const agent = new Agent({
      model,
      tools: boundTools as any,
      systemPrompt,
      streamFn,
      approvalMode,
      onApproval: this.decisions.createApprovalHandler(sessionId),
    });

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
    const shouldGenerateTitle = isGenericSessionTitle(session.header.title) && session.messages.length === 0;
    this.sessionStorage.appendMessage(sessionId, userMessage);

    if (shouldGenerateTitle) {
      void generateSessionTitle(prompt, model, streamFn)
        .then((title) => {
          if (!title) return;
          const latest = this.sessionStorage.loadSession(sessionId);
          if (latest && isGenericSessionTitle(latest.header.title)) {
            this.sessionStorage.updateTitle(sessionId, title);
            hub.broadcast({ type: "sessionTitleUpdated", title });
          }
        })
        .catch(() => {});
    }
    agent.loadHistory(session.messages);

    this.sessionStorage.markSessionNeedsRepair(sessionId);
    this.sessionStorage.updateSessionStatus(sessionId, "working");
    const runPersistenceId = randomUUID();
    let toolBatchNumber = 0;
    let toolResultsPersistenceId: string | null = null;
    const pendingToolCalls = new Map<string, { name: string; args: any }>();

    try {
      const eventStream = agent.run(prompt, abortController.signal, attachments);
      let runError: string | null = null;

      for await (const event of eventStream) {
        if (event.type === "toolExecutionStart") {
          toolResultsPersistenceId = `tool-results:${runPersistenceId}:${toolBatchNumber++}`;
          for (const call of event.calls) {
            pendingToolCalls.set(call.id, { name: call.name, args: call.arguments });
          }
        }
        if (event.type === "modelStreamEnd" && event.turn) {
          this.sessionStorage.appendMessage(sessionId, event.turn);
        }
        if (event.type === "toolExecutionResult" && toolResultsPersistenceId) {
          this.sessionStorage.upsertToolResult(sessionId, toolResultsPersistenceId, event.result);
          const callInfo = pendingToolCalls.get(event.result.toolCallId);
          if (callInfo) {
            extractAndRecordFileChange(this.sessionStorage, sessionId, callInfo.name, callInfo.args, event.result.isError);
          }
        }
        if (event.type === "toolExecutionEnd") {
          for (const result of event.results) {
            if (!toolResultsPersistenceId) continue;
            this.sessionStorage.upsertToolResult(sessionId, toolResultsPersistenceId, result);
            const callInfo = pendingToolCalls.get(result.toolCallId);
            if (callInfo) {
              extractAndRecordFileChange(this.sessionStorage, sessionId, callInfo.name, callInfo.args, result.isError);
            }
          }
          toolResultsPersistenceId = null;
          pendingToolCalls.clear();
        }

        if (event.type === "subagentStart") {
          this.sessionStorage.upsertSubagentStart(sessionId, event);
        } else if (event.type === "subagentActivity") {
          this.sessionStorage.appendSubagentActivity(sessionId, event);
        } else if (event.type === "subagentEnd") {
          this.sessionStorage.completeSubagent(sessionId, event);
        }

        if (event.type === "askQuestion" || event.type === "permissionRequest") {
          this.sessionStorage.updateSessionStatus(sessionId, "needs_attention");
        }

        if (isAttentionEvent(event)) {
          notificationService.push(attentionNotification(sessionId, event));
        } else if (isDoneEvent(event) && !runError) {
          notificationService.push(doneNotification(sessionId));
        }

        // Compaction is internal LLM context memory management; do not broadcast to user UI
        if (event.type !== "compaction") {
          hub.broadcast(event);
        }

        if (event.type === "error") {
          runError = event.error?.message ?? "Unknown agent error";
        }
      }

      await eventStream.result();

      if (runError) {
        this.sessionStorage.appendMessage(sessionId, {
          role: "assistant",
          content: [{ type: "text", text: `Error: ${runError}` }],
        });
      }
      this.sessionStorage.updateSessionStatus(sessionId, runError ? "needs_attention" : "done");
    } catch (err) {
      const isAbort =
        abortController.signal.aborted ||
        (err instanceof Error && err.name === "AbortError") ||
        (err instanceof Error && err.message === "This operation was aborted.");
      if (isAbort) {
        this.sessionStorage.updateSessionStatus(sessionId, "done");
      } else {
        this.sessionStorage.updateSessionStatus(sessionId, "needs_attention");
        const errorMsg = extractErrorMessage(err);
        this.sessionStorage.appendMessage(sessionId, {
          role: "assistant",
          content: [{ type: "text", text: `Error: ${errorMsg}` }],
        });
        throw err;
      }
    } finally {
      const activeTodos = this.todoLists.get(sessionId) ?? this.sessionStorage.getSessionTodos(sessionId);
      if (activeTodos.length > 0 && activeTodos.every((item) => item.status === "completed")) {
        this.sessionStorage.clearSessionTodos(sessionId);
        this.todoLists.delete(sessionId);
        hub.broadcast({ type: "todoUpdate", items: [], action: "updated" });
      }

      this.sessionStorage.repairSession(sessionId);
      hub.broadcast(
        abortController.signal.aborted
          ? { type: "aborted", reason: "Run was aborted." }
          : { type: "done" },
      );
      this.decisions.rejectAllForSession(sessionId, "Run ended");
    }
  }

  answerQuestion(sessionId: string, requestId: string, answer: string | string[]): boolean {
    return this.decisions.answerQuestion(sessionId, requestId, answer);
  }

  approvePermission(sessionId: string, requestId: string, allow: boolean): boolean {
    return this.decisions.approvePermission(sessionId, requestId, allow);
  }

  abortRun(sessionId: string, options?: { clearQueue?: boolean }): boolean {
    const controller = RunService.activeRuns.get(sessionId);
    if (!controller) return false;

    controller.abort();
    this.decisions.rejectAllForSession(sessionId, "Run aborted");
    if (options?.clearQueue) {
      // Plain Stop means stop everything: a staged next turn predicated on
      // the aborted turn must not fire. (Steer calls abortRun without the
      // flag, so the prompt it just staged is unaffected.)
      RunService.pendingNextTurn.delete(sessionId);
      this.sessionStorage.clearQueuedPrompt(sessionId);
      this.hubs.get(sessionId)?.broadcast({ type: "queueUpdated", queuedPrompt: null });
    }
    // Do NOT delete activeRuns here — the run's turn loop owns that
    // lifecycle. Deleting immediately would let a second `runAgentStream`
    // start before the first settles (shared hub/session race).
    return true;
  }
}
