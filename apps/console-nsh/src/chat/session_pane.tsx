// One chat tab's content: the transcript plus composer, backed by the real
// session. Ported from the desktop's per-pane transcript wiring — the pane
// loads the session's message tail, streams runs through the native bridge
// (`Console.runPrompt`), and reconciles with the canonical history when a
// run settles (desktop `run.rs`'s `wait_until_settled` step).
//
// The interactive layer is the desktop's pending-permission /
// pending-question machinery: `permissionRequest` and `askQuestion` events
// pin their cards between the transcript and composer, and the card's
// answer goes back through `Console.approvePermission` /
// `Console.answerQuestion`. The header row above the composer mirrors the
// desktop's status/model pill; prompt history feeds the composer's Up-arrow
// recall.
//
// Components run once in nsh, so the controller below is created once per
// mount and everything dynamic reads its signals inside JSX.

import { signal, Text, View, token, type JSX } from "nsh";
import { Input } from "nsh/base";
import { Console } from "console-nsh/native";
import { Composer } from "../common/composer";
import { TranscriptView, createTranscriptState } from "./transcript_view";
import {
  PermissionInteractionCard,
  QuestionInteractionCard,
} from "./interaction_card";
import { applyRunEvent, type ChatRunEvent } from "./run_events";
import type { AgentMessage, UserMessage } from "./transcript_model";

export interface ChatController {
  state: ReturnType<typeof createTranscriptState>;
  /// Sends a prompt as a new run. Optimistically appends the user message,
  /// streams the run's events into the transcript, then reconciles with the
  /// server's canonical history once the run settles.
  send(prompt: string): Promise<void>;
  /// Reloads the tail page (the canonical history after a settle or an
  /// external change).
  refresh(): Promise<void>;
  /// Answers the pending permission (allow) or denies it.
  answerPermission(allow: boolean): Promise<void>;
  /// Answers the pending question, or skips it.
  answerQuestion(answer: string | string[]): Promise<void>;
  skipQuestion(): Promise<void>;
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

function decodeArgs(base64: string | undefined): unknown {
  if (base64 == null) return null;
  try {
    return JSON.parse(atob(base64));
  } catch {
    return null;
  }
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
      const detail = JSON.parse(await Console.chatMessages(baseUrl, sessionId)) as SessionDetailJson;
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

  async function send(prompt: string): Promise<void> {
    const trimmed = prompt.trim();
    if (trimmed === "") return;
    // Optimistic user row; canonical ids land with the settle refresh.
    state.pushMessage({
      type: "user",
      content: trimmed,
      createdAt: Math.floor(Date.now() / 1000),
    });
    state.beginStreaming();
    try {
      await Console.runPrompt(
        baseUrl,
        sessionId,
        { prompt: trimmed },
        (event: unknown) => {
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
                argsBase64:
                  typeof request["args"] === "string" ? request["args"] : undefined,
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
        },
      );
      // The run's stream ended: swap the streamed shadow for the server's
      // canonical messages (desktop `run.rs`'s settle step). A shorter
      // canonical list than what we streamed means the server has not
      // flushed yet — adopt ids instead of clobbering live rows.
      const detail = JSON.parse(await Console.chatMessages(baseUrl, sessionId)) as SessionDetailJson;
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
      await Console.approvePermission(baseUrl, sessionId, request.requestId, allow);
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
      await Console.answerQuestion(baseUrl, sessionId, request.requestId, answer);
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

  void load();

  return {
    state,
    send,
    refresh: load,
    answerPermission,
    answerQuestion,
    skipQuestion,
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

/// The header row above the composer: the session's status and the model
/// chip (the desktop's composer footer, minus the picker — the model menu
/// lands with the model catalog port).
function SessionHeaderRow(props: { controller: ChatController }) {
  const controller = props.controller;
  const status = controller.sessionStatus();
  const working = status === "working";
  const label = working ? "Working" : status;
  return (
    <View class="flex-none flex-row items-center gap-2 px-3 py-1">
      <View
        class="h-1.5 w-1.5 rounded-full"
        style={{ backgroundColor: token(working ? "warning" : "muted-foreground") }}
      />
      <Text class="text-[11px] text-muted-foreground">{label}</Text>
      {controller.sessionModel() !== "" ? (
        <Text class="max-w-[220px] truncate rounded border border-border px-1.5 py-[1px] text-[10px] text-muted-foreground">
          {controller.sessionProvider() !== ""
            ? `${controller.sessionProvider()} · ${controller.sessionModel()}`
            : controller.sessionModel()}
        </Text>
      ) : null}
    </View>
  );
}

/// The custom-answer input inside the question card (a one-line input bound
/// to a signal; the card reads it through the `customAnswer` thunk). One
/// shared draft: the server asks one question per run, so only one card is
/// on screen at a time.
const questionCustomAnswer = signal("");
const questionSelected = signal<string[]>([]);

function QuestionCustomInput(props: { controller: ChatController }): JSX.Element {
  return (
    <Input
      multiline={false}
      class="px-2.5 py-1.5 text-[12px] text-foreground"
      placeholder="Custom answer…"
      value={questionCustomAnswer()}
      onChange={(e: { value: string }) => questionCustomAnswer.set(e.value)}
      onSubmit={() => {
        if (questionCustomAnswer().trim() !== "") {
          void props.controller.answerQuestion(questionCustomAnswer().trim());
          questionCustomAnswer.set("");
        }
      }}
    />
  );
}

/// One chat tab's rendered surface: the transcript, the trailing error line,
/// the pinned interaction cards, and the composer. The controller (created
/// once per mount) owns the state.
export function SessionPane(props: { controller: ChatController }) {
  const controller = props.controller;
  const draft = signal("");
  return (
    <View class="flex-1 flex-col">
      {controller.error() != null ? (
        <View class="flex-none px-3 py-1">
          <Text class="text-[11px] text-red-500">{controller.error()}</Text>
        </View>
      ) : null}
      <TranscriptView state={controller.state} />
      {(() => {
        const permission = controller.pendingPermission();
        if (permission != null) {
          return (
            <View class="flex-none p-2">
              <PermissionInteractionCard
                request={{
                  toolName: permission.toolName,
                  args: decodeArgs(permission.argsBase64),
                  reason: permission.reason,
                }}
                submitting={controller.submitting()}
                onApprove={(allow) => void controller.answerPermission(allow)}
              />
            </View>
          );
        }
        const question = controller.pendingQuestion();
        if (question != null) {
          return (
            <View class="flex-none p-2">
              <QuestionInteractionCard
                request={{
                  question: question.question,
                  options: question.options ?? [],
                  isMultiSelect: question.isMultiSelect ?? false,
                }}
                selected={() => [...questionSelected()]}
                customAnswer={() => questionCustomAnswer()}
                customInput={<QuestionCustomInput controller={controller} />}
                isLast={true}
                submitting={controller.submitting()}
                onAnswer={(answer) => {
                  questionSelected.set([]);
                  questionCustomAnswer.set("");
                  void controller.answerQuestion(answer);
                }}
                onSelect={(option) => {
                  const current = [...questionSelected()];
                  if (question.isMultiSelect ?? false) {
                    const at = current.indexOf(option);
                    if (at >= 0) current.splice(at, 1);
                    else current.push(option);
                    questionSelected.set(current);
                  } else {
                    questionSelected.set([option]);
                  }
                }}
                onSkip={() => void controller.skipQuestion()}
              />
            </View>
          );
        }
        return null;
      })()}
      <SessionHeaderRow controller={controller} />
      <View class="flex-none border-t border-border">
        <Composer
          value={draft()}
          onChange={(text: string) => draft.set(text)}
          onSend={() => {
            const text = draft();
            draft.set("");
            void controller.send(text);
          }}
          runState={controller.state.isStreaming() ? "running" : "ready"}
          promptHistory={controller.promptHistory()}
        />
      </View>
    </View>
  );
}
