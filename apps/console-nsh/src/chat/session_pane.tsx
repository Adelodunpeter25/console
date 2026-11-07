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
import { Icon } from "../common/icons";
import { TranscriptView } from "./transcript_view";
import { changesSummaryFrom, changesSummaryIsEmpty } from "./change_chips";
import {
  PermissionInteractionCard,
  QuestionInteractionCard,
} from "./interaction_card";
import type { ChatController } from "../services/chat_controller";
import { catalogStore } from "../stores/catalog_store";

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
  baseUrl: string;
  onOpenSession?: (sessionId: string, title: string) => void;
  /// Opens the diff tab for one changed file (a change chip's press).
  onOpenChange?: (path: string, diffText: string | null) => void;
  /// Opens the session's changes-review tab (the header's review chip).
  onOpenReview?: () => void;
  /// Previews a message image full-screen (the bubble's press).
  onPreviewImage?: (uri: string) => void;
}) {
  const controller = props.controller;
  const draft = signal("");
  const catalog = catalogStore(props.baseUrl);
  void catalog.load();
  return (
    <View class="flex-1 flex-col">
      {controller.error() != null ? (
        <View class="flex-none px-3 py-1">
          <Text class="text-[11px] text-red-500">{controller.error()}</Text>
        </View>
      ) : null}
      {/* flex-1 wrapper: TranscriptView fills its height (h-full List); a
          bare h-full child would take the whole pane and clip the composer. */}
      <View class="min-h-0 flex-1 flex-col">
        <TranscriptView
          state={controller.state}
          onFork={(messageId: string) => {
            void (async () => {
              const forked = await controller.fork(messageId);
              if (forked != null) props.onOpenSession?.(forked, "Fork");
            })();
          }}
          onOpenChange={props.onOpenChange}
          onPreviewImage={props.onPreviewImage}
        />
      </View>
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
      {/* The turn's changes pill: GPUI's `changes_pill` — "N files changed
          +A -D" over the raised surface; pressing opens the review tab. */}
      {(() => {
        const summary = changesSummaryFrom(controller.state.sessionChanges());
        if (props.onOpenReview == null || changesSummaryIsEmpty(summary)) {
          return null;
        }
        return (
          <View class="flex-none flex-row px-3 pb-1">
            <View
              class="flex-row items-center gap-1.5 rounded-full bg-popover px-2.5 py-[5px] hover:bg-muted"
              onPress={props.onOpenReview}
              accessibilityLabel="Open changes review"
              role="button"
            >
              <Icon name="FileDiff" size={13} color={token("muted-foreground")} />
              <Text class="text-[12px] font-medium text-muted-foreground">
                {`${summary.files} file${summary.files === 1 ? "" : "s"} changed`}
              </Text>
              {summary.additions > 0 ? (
                <Text class="text-[12px] font-semibold text-[#62c987]">
                  {`+${summary.additions}`}
                </Text>
              ) : null}
              {summary.deletions > 0 ? (
                <Text class="text-[12px] font-semibold text-destructive">
                  {`-${summary.deletions}`}
                </Text>
              ) : null}
            </View>
          </View>
        );
      })()}
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
          providers={catalog.providers() ?? undefined}
          selectedModel={catalog.selected()}
          favorites={catalog.favorites()}
          onModelSelect={(provider: string, modelId: string) =>
            catalog.selectModel(provider, modelId)
          }
          onFavorite={(provider: string, modelId: string) =>
            void catalog.toggleFavorite(provider, modelId)
          }
        />
      </View>
    </View>
  );
}
