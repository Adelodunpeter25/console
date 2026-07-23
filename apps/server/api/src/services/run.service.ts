/**
 * Agent Run Execution Service.
 * Manages active run controllers, system prompt building, and agent turn execution with real-time turn persistence.
 */
import { Agent } from "../../../agent/src/service/agent.js";
import { allTools } from "../../../agent/src/tools/index.js";
import { SqliteSessionStorage } from "../../../agent/src/session/storage.js";
import { buildSystemPrompt } from "../../../agent/src/systemprompt/builder.js";
import { createAntigravityStreamFn } from "../../../providers/src/antigravity/stream-fn.js";
import { geminiStreamFn } from "../../../providers/src/gemini/stream-fn.js";
import type { AgentSessionEvent, Model } from "../../../agent/src/types/index.js";
import type { RunPromptDto } from "../types/index.js";

export class RunService {
  private sessionStorage = new SqliteSessionStorage();
  private activeRuns = new Map<string, AbortController>();

  /**
   * Execute an agent run, streaming lifecycle events to onEvent callback.
   * Persists completed turns incrementally so data is never lost during network drops or crashes.
   */
  async runAgentStream(
    sessionId: string,
    dto: RunPromptDto,
    onEvent: (event: AgentSessionEvent) => Promise<void> | void,
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
    const approvalMode = dto.approvalMode || "always-ask";

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

    const agent = new Agent({
      model,
      tools: [...allTools] as any,
      systemPrompt,
      streamFn,
      approvalMode,
    });

    agent.loadHistory(session.messages);

    const abortController = new AbortController();
    this.activeRuns.set(sessionId, abortController);

    try {
      const eventStream = agent.run(prompt, abortController.signal);

      for await (const event of eventStream) {
        await onEvent(event);

        // Incremental turn persistence on modelStreamEnd
        if (event.type === "modelStreamEnd") {
          this.sessionStorage.appendMessages(sessionId, [event.turn]);
        }
      }

      // Final persistence sync for all messages
      const updatedMessages = await eventStream.result();
      this.sessionStorage.appendMessages(sessionId, updatedMessages.slice(session.messages.length));
    } finally {
      this.activeRuns.delete(sessionId);
    }
  }

  /**
   * Abort an active run for a session.
   */
  abortRun(sessionId: string): boolean {
    const controller = this.activeRuns.get(sessionId);
    if (!controller) return false;

    controller.abort();
    this.activeRuns.delete(sessionId);
    return true;
  }
}
