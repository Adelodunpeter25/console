/**
 * Agent Run Execution Service.
 * Manages active run controllers, system prompt building, and agent turn execution with real-time turn persistence.
 */
import { Agent } from "../../../agent/src/service/agent.js";
import { allTools } from "../../../agent/src/tools/index.js";
import { createAskTool } from "../../../agent/src/tools/ask.js";
import { SqliteSessionStorage } from "../../../agent/src/session/storage.js";
import { buildSystemPrompt } from "../../../agent/src/systemprompt/builder.js";
import { createAntigravityStreamFn } from "../../../providers/src/antigravity/stream-fn.js";
import { geminiStreamFn } from "../../../providers/src/gemini/stream-fn.js";
import type { AgentSessionEvent, ApprovalMode, Model } from "../../../agent/src/types/index.js";
import type { RunPromptDto } from "../types/index.js";

export class RunService {
  private sessionStorage = new SqliteSessionStorage();
  private activeRuns = new Map<string, AbortController>();
  /** Pending question answers keyed by requestId. */
  private pendingQuestions = new Map<
    string,
    { sessionId: string; resolve: (answer: string | string[]) => void; reject: (err: unknown) => void }
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
    if (this.activeRuns.has(sessionId)) {
      throw new Error(`Session '${sessionId}' already has an active run.`);
    }

    const abortController = new AbortController();
    this.activeRuns.set(sessionId, abortController);
    try {
      await this.runAgentStreamInternal(sessionId, dto, onEvent, abortController);
    } finally {
      this.activeRuns.delete(sessionId);
    }
  }

  private async runAgentStreamInternal(
    sessionId: string,
    dto: RunPromptDto,
    onEvent: (event: AgentSessionEvent) => Promise<void> | void,
    abortController: AbortController,
  ): Promise<void> {
    const prompt = dto.prompt.trim();

    let session = this.sessionStorage.loadSession(sessionId);
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

    const provider = dto.provider || session.header.provider || "antigravity";
    const modelId = dto.modelId || session.header.modelId || "gemini-2.5-pro";
    // Use the approvalMode from the request; fall back to the persisted session value,
    // then to "always-ask" as the safe default. Never silently run without a mode.
    const approvalMode = (dto.approvalMode || session.header.approvalMode || "always-ask") as ApprovalMode;

    // Persist the chosen approvalMode to the DB so it survives reloads.
    this.sessionStorage.updateApprovalMode(sessionId, approvalMode);

    const model: Model = {
      id: modelId,
      provider: provider as any,
      contextWindow: 128_000,
    };

    const streamFn = provider === "gemini" ? geminiStreamFn : createAntigravityStreamFn();

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
      });
    });

    const tools = allTools.map((tool) => (tool.name === "ask" ? askTool : tool));

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
        });
      },
    });

    // Persist the user message immediately so it survives crashes, errors, and
    // session switches even if the run never completes. The agent also appends
    // this prompt to its internal history; we skip the duplicate at the end.
    this.sessionStorage.appendMessage(sessionId, { role: "user", content: prompt });

    agent.loadHistory(session.messages);

    this.sessionStorage.updateSessionStatus(sessionId, "working");

    try {
      const eventStream = agent.run(prompt, abortController.signal);

      // Track run failures reported by the agent loop (stream errors, max
      // turns, provider failures) so they can be persisted as error messages.
      let runError: string | null = null;

      for await (const event of eventStream) {
        await onEvent(event);

        // Mark needs_attention when the agent asks a question or requests permission
        if (event.type === "askQuestion" || event.type === "permissionRequest") {
          this.sessionStorage.updateSessionStatus(sessionId, "needs_attention");
        }

        if (event.type === "error") {
          runError = event.error?.message ?? "Unknown agent error";
        }
      }

      // Persist all new messages in one batch so they share a single
      // created_at timestamp and are ordered by insertion (rowid) — this
      // preserves the correct conversation order (user → assistant → tool
      // result → assistant → …) on reload.
      const updatedMessages = await eventStream.result();
      const persistedBefore = session.messages.length;
      const newMessages = updatedMessages
        .slice(persistedBefore)
        // The user prompt is already persisted above — skip it to avoid a duplicate row.
        .filter((m) => !(m.role === "user" && m.content === prompt));
      this.sessionStorage.appendMessages(sessionId, newMessages);

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
   * Abort an active run for a session.
   */
  abortRun(sessionId: string): boolean {
    const controller = this.activeRuns.get(sessionId);
    if (!controller) return false;

    controller.abort();
    return true;
  }
}
