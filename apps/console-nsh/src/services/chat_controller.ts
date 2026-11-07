// The chat controller: the stateful service behind one session's chat tab.
// Moved out of `session_pane.tsx` so components render, and this layer
// talks to the backend. Controllers live in a module-level cache keyed by
// session id, so switching tabs back keeps the loaded transcript instead of
// reloading (the desktop keeps one transcript entity per session the same
// way); the launcher streams its draft into the same controller the pane
// renders.

import { signal } from "nsh";
import { createTranscriptState } from "../chat/transcript_view";
import { applyRunEvent, type ChatRunEvent } from "../chat/run_events";
import type { AgentMessage, UserMessage } from "../chat/transcript_model";
import type { SelectedModel } from "../stores/catalog_store";
import { backend } from "./backend";

export interface ChatController {
  state: ReturnType<typeof createTranscriptState>;
  /// Sends a prompt as a new run. Optimistically appends the user message,
  /// streams the run's events into the transcript, then reconciles with the
  /// server's canonical history once the run settles.
  send(prompt: string, model?: SelectedModel | null): Promise<void>;
  /// Reloads the tail page (the canonical history after a settle or an
  /// external change).
  refresh(): Promise<void>;
  /// Answers the pending permission (allow) or denies it.
  answerPermission(allow: boolean): Promise<void>;
  /// Answers the pending question, or skips it.
  answerQuestion(answer: string | string[]): Promise<void>;
  skipQuestion(): Promise<void>;
  /// Forks the chat at one of its messages; resolves the forked session id.
  fork(messageId: string): Promise<string | null>;
  error: () => string | null;
  /// The session header row's inputs, from the last canonical load.
  sessionStatus: () => string;
  sessionModel: () => string;
  sessionProvider: () => string;
  /// Past prompts, oldest first (the composer's history recall).
  promptHistory: () => string[];
  /// The pinned interaction cards' current requests (null = none pending).
  pendingPermission: () => PermissionRequestJson | null;
  pendingQuestion: () => AskQuestionJson | null;
  submitting: () => boolean;
}

// The wire shapes (pbjson camelCase; `args` is base64 bytes).
export interface PermissionRequestJson {
  requestId: string;
  toolName: string;
  argsBase64?: string;
  reason?: string | null;
}
export interface AskQuestionJson {
  requestId: string;
  question: string;
  options?: string[] | null;
  isMultiSelect?: boolean | null;
  skippable?: boolean | null;
}

interface SessionDetailJson {
  header?: {
    status?: string;
    modelId?: string;
    provider?: string;
  };
  messages: AgentMessage[];
  hasMore?: boolean;
  nextCursor?: number | null;
}

export function createChatController(baseUrl: string, sessionId: string): ChatController {
  const state = createTranscriptState();
  const loadError = signal<string | null>(null);
  const sessionStatus = signal("idle");
  const sessionModel = signal("");
  const sessionProvider = signal("");
  const promptHistory = signal<string[]>([]);
  const pendingPermission = signal<PermissionRequestJson | null>(null);
  const pendingQuestion = signal<AskQuestionJson | null>(null);
  const submitting = signal(false);

  async function load(): Promise<void> {
    try {
      const detail = JSON.parse(await backend.chatMessages(baseUrl, sessionId)) as SessionDetailJson;
      state.setMessages(detail.messages ?? []);
      state.setPagination(detail.hasMore ?? false, detail.nextCursor ?? null);
      sessionStatus.set(detail.header?.status ?? "idle");
      sessionModel.set(detail.header?.modelId ?? "");
      sessionProvider.set(detail.header?.provider ?? "");
      promptHistory.set(
        (detail.messages ?? [])
          .filter((m): m is UserMessage => m.type === "user")
          .map((m) => m.content)
          .reverse(),
      );
      loadError.set(null);
    } catch (err) {
      loadError.set(err instanceof Error ? err.message : String(err));
    }
  }

  async function send(prompt: string, model?: SelectedModel | null): Promise<void> {
    const trimmed = prompt.trim();
    if (trimmed === "") return;
    // The picker's choice rides with the run (desktop parity: the floating
    // composer's card model is pushed onto the run, not the pane's).
    const payload: Record<string, unknown> = { prompt: trimmed };
    if (model != null) {
      payload.modelId = model.modelId;
      payload.provider = model.provider;
    }
    // Optimistic user row; canonical ids land with the settle refresh.
    state.pushMessage({
      type: "user",
      content: trimmed,
      createdAt: Math.floor(Date.now() / 1000),
    });
    state.beginStreaming();
    try {
      await backend.runPrompt(baseUrl, sessionId, payload, (event: unknown) => {
        const ev = event as ChatRunEvent;
        applyRunEvent(state, ev, {
          onPermission: (request) => {
            if (request == null) {
              pendingPermission.set(null);
              return;
            }
            pendingQuestion.set(null);
            pendingPermission.set({
              requestId: String(request["requestId"] ?? ""),
              toolName: String(request["toolName"] ?? ""),
              argsBase64: typeof request["args"] === "string" ? request["args"] : undefined,
              reason: typeof request["reason"] === "string" ? request["reason"] : null,
            });
          },
          onQuestion: (request) => {
            if (request == null) {
              pendingQuestion.set(null);
              return;
            }
            pendingPermission.set(null);
            pendingQuestion.set({
              requestId: String(request["requestId"] ?? ""),
              question: String(request["question"] ?? ""),
              options: Array.isArray(request["options"]) ? (request["options"] as string[]) : [],
              isMultiSelect: (request["isMultiSelect"] as boolean | undefined) ?? false,
              skippable: (request["skippable"] as boolean | undefined) ?? true,
            });
          },
        });
      });
      // The run's stream ended: swap the streamed shadow for the server's
      // canonical messages (desktop `run.rs`'s settle step). A shorter
      // canonical list than what we streamed means the server has not
      // flushed yet — adopt ids instead of clobbering live rows.
      const detail = JSON.parse(await backend.chatMessages(baseUrl, sessionId)) as SessionDetailJson;
      if (detail.messages.length >= state.messages().length) {
        state.setMessages(detail.messages);
      } else {
        state.adoptUserMessageIds(detail.messages);
      }
      state.setPagination(detail.hasMore ?? false, detail.nextCursor ?? null);
      sessionStatus.set(detail.header?.status ?? "idle");
      promptHistory.set(
        (detail.messages ?? [])
          .filter((m): m is UserMessage => m.type === "user")
          .map((m) => m.content)
          .reverse(),
      );
      loadError.set(null);
    } catch (err) {
      state.finishStreaming();
      loadError.set(err instanceof Error ? err.message : String(err));
    }
  }

  async function answerPermission(allow: boolean): Promise<void> {
    const request = pendingPermission();
    if (request == null || submitting()) return;
    submitting.set(true);
    try {
      await backend.approvePermission(baseUrl, sessionId, request.requestId, allow);
      pendingPermission.set(null);
    } catch (err) {
      loadError.set(err instanceof Error ? err.message : String(err));
    } finally {
      submitting.set(false);
    }
  }

  async function answerQuestion(answer: string | string[]): Promise<void> {
    const request = pendingQuestion();
    if (request == null || submitting()) return;
    submitting.set(true);
    try {
      await backend.answerQuestion(baseUrl, sessionId, request.requestId, answer);
      pendingQuestion.set(null);
    } catch (err) {
      loadError.set(err instanceof Error ? err.message : String(err));
    } finally {
      submitting.set(false);
    }
  }

  async function skipQuestion(): Promise<void> {
    await answerQuestion("");
  }

  async function fork(messageId: string): Promise<string | null> {
    try {
      const header = await backend.forkSession(baseUrl, sessionId, messageId);
      return header.id;
    } catch (err) {
      loadError.set(err instanceof Error ? err.message : String(err));
      return null;
    }
  }

  void load();

  return {
    state,
    send,
    refresh: load,
    answerPermission,
    answerQuestion,
    skipQuestion,
    fork,
    error: loadError,
    sessionStatus: sessionStatus,
    sessionModel: sessionModel,
    sessionProvider: sessionProvider,
    promptHistory: promptHistory,
    pendingPermission: pendingPermission,
    pendingQuestion: pendingQuestion,
    submitting: submitting,
  };
}

const chatControllers = new Map<string, ChatController>();

/// The controller for one session's chat tab, creating it on first use.
export function chatController(baseUrl: string, sessionId: string): ChatController {
  let controller = chatControllers.get(sessionId);
  if (controller == null) {
    controller = createChatController(baseUrl, sessionId);
    chatControllers.set(sessionId, controller);
  }
  return controller;
}
