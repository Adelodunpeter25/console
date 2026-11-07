// One chat tab's content: the transcript plus composer, backed by the real
// session. Ported from the desktop's per-pane transcript wiring — the pane
// renders, and its controller (in `services/chat_controller.ts`) loads the
// session's message tail, streams runs, and reconciles with the canonical
// history when a run settles (desktop `run.rs`'s `wait_until_settled`
// step).
//
// The interactive layer is the desktop's pending-permission /
// pending-question machinery: `permissionRequest` and `askQuestion` events
// pin their cards between the transcript and composer. The header row above
// the composer mirrors the desktop's status/model pill; prompt history
// feeds the composer's Up-arrow recall.
//
// Components run once in nsh, so the controller is created once per
// session and everything dynamic reads its signals inside JSX.

import { signal, Text, View, token, type JSX } from "nsh";
import { Input } from "nsh/base";
import { Composer } from "../common/composer";
import { TranscriptView } from "./transcript_view";
import {
  PermissionInteractionCard,
  QuestionInteractionCard,
} from "./interaction_card";
import type { ChatController } from "../services/chat_controller";

function decodeArgs(base64: string | undefined): unknown {
  if (base64 == null) return null;
  try {
    return JSON.parse(atob(base64));
  } catch {
    return null;
  }
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
/// once per mount) owns the state. `onOpenSession` opens a forked session
/// as a new tab (the desktop's fork behaviour).
export function SessionPane(props: {
  controller: ChatController;
  onOpenSession?: (sessionId: string, title: string) => void;
}) {
  const controller = props.controller;
  const draft = signal("");
  return (
    <View class="flex-1 flex-col">
      {controller.error() != null ? (
        <View class="flex-none px-3 py-1">
          <Text class="text-[11px] text-red-500">{controller.error()}</Text>
        </View>
      ) : null}
      <TranscriptView
        state={controller.state}
        onFork={(messageId: string) => {
          void (async () => {
            const forked = await controller.fork(messageId);
            if (forked != null) props.onOpenSession?.(forked, "Fork");
          })();
        }}
      />
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
