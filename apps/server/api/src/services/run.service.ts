/**
 * Agent Run Execution Service.
 * Coordinates active run controllers, streaming lifecycle events, and turn persistence.
 */
import { Agent } from "@/agent/src/service/agent.js";
import { type TodoItem } from "@/agent/src/tools/todo.js";
import { SqliteSessionStorage } from "@/agent/src/session/storage.js";
import { buildSystemPrompt } from "@/agent/src/systemprompt/builder.js";
import { findModelInProvider, getProvider } from "@/agent/src/commands/provider-registry.js";
import type {
  AgentSessionEvent,
  ApprovalMode,
  ImagePart,
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

export class RunService {
  private sessionStorage = new SqliteSessionStorage();
  private static activeRuns = new Map<string, AbortController>();
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

    if (session.messages.length > 0 && this.sessionStorage.repairSession(sessionId)) {
      session = this.sessionStorage.loadSession(sessionId) ?? session;
    }

    const provider = dto.provider || session.header.provider || "antigravity";
    const modelId = dto.modelId || session.header.modelId || "gemini-2.5-pro";
    const catalogModel = findModelInProvider(provider, modelId);
    if (dto.attachments && dto.attachments.length > 0 && catalogModel?.supportsImages === false) {
      throw new Error(`The selected model '${modelId}' does not support image attachments.`);
    }

    const approvalMode = (dto.approvalMode ||
      session.header.approvalMode ||
      "always-ask") as ApprovalMode;

    this.sessionStorage.updateModel(sessionId, modelId, provider);
    this.sessionStorage.updateApprovalMode(sessionId, approvalMode);

    const model = buildRunModel(provider, modelId);
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
    this.sessionStorage.appendMessage(sessionId, userMessage);
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

        if (event.type === "askQuestion" || event.type === "permissionRequest") {
          this.sessionStorage.updateSessionStatus(sessionId, "needs_attention");
        }

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

  abortRun(sessionId: string): boolean {
    const controller = RunService.activeRuns.get(sessionId);
    if (!controller) return false;

    controller.abort();
    this.decisions.rejectAllForSession(sessionId, "Run aborted");
    RunService.activeRuns.delete(sessionId);
    return true;
  }
}
