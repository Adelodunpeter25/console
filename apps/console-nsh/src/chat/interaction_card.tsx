// The interaction cards the transcript shows while the agent awaits the
// user, ported from console-ui's `chat/interaction_card.rs`: the permission
// prompt (allow / deny) and the multi-option question card (select options,
// optional custom answer, submit / skip).
//
// Theme notes: the desktop's `theme.success` has no NSH palette token, so the
// allow affordances read as `primary` (the `pill_popovers.tsx` convention).

import { Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Icon } from "../common/icons";
import { MarkdownBlocks, METRICS } from "./markdown/render";

/// console-core's `PermissionRequest` wire shape.
export interface PermissionRequest {
  toolName: string;
  args: unknown;
  reason?: string | null;
  requiresUpgrade?: boolean | null;
}

/// console-core's `AskQuestionRequest` wire shape.
export interface AskQuestionRequest {
  question: string;
  options?: string[] | null;
  isMultiSelect?: boolean | null;
}

export function PermissionInteractionCard(props: {
  request: PermissionRequest;
  submitting: boolean;
  onApprove: (approved: boolean) => void;
}) {
  const req = props.request;
  const requiresUpgrade = req.requiresUpgrade ?? false;
  return (
    <View
      class="w-full max-w-[728px] flex-col gap-[10px] rounded-xl border bg-card p-[14px] shadow-lg"
      style={{ borderColor: token("warning") }}
    >
      {/* Header */}
      <View class="flex-row items-center gap-2">
        <Icon name="TriangleAlert" size={14} color={token("warning")} />
        <Text class="text-[13px] font-semibold text-foreground">
          {requiresUpgrade
            ? `Upgrade required: ${req.toolName}`
            : `Permission required: ${req.toolName}`}
        </Text>
      </View>
      {/* Optional reason */}
      {req.reason ? (
        <View class="pl-6">
          <MarkdownBlocks source={req.reason} metrics={METRICS.BODY} />
        </View>
      ) : null}
      {/* Tool args JSON preview */}
      <View class="ml-6 max-h-[140px] overflow-hidden rounded-md border border-border bg-muted p-2">
        <Text class="font-mono text-[11px] text-muted-foreground">
          {prettyJson(req.args)}
        </Text>
      </View>
      {/* Actions */}
      <View class="flex-row items-center gap-2 pl-6">
        <View
          class="flex-row items-center gap-1.5 rounded-md border border-primary/35 bg-primary/15 px-3 py-[6px] hover:bg-primary/25"
          role="button"
          accessibilityLabel={requiresUpgrade ? "Allow once" : "Allow"}
          onPress={() => props.onApprove(true)}
        >
          <Icon name="Check" size={11} color={token("primary")} />
          <Text class="text-[12px] font-medium text-primary">
            {requiresUpgrade ? "Allow once" : "Allow"}
          </Text>
        </View>
        <View
          class="flex-row items-center gap-1.5 rounded-md border border-destructive/35 bg-destructive/15 px-3 py-[6px] hover:bg-destructive/25"
          role="button"
          accessibilityLabel="Deny"
          onPress={() => props.onApprove(false)}
        >
          <Icon name="X" size={11} color={token("destructive")} />
          <Text class="text-[12px] font-medium text-destructive">Deny</Text>
        </View>
      </View>
    </View>
  );
}

export function QuestionInteractionCard(props: {
  request: AskQuestionRequest;
  /// The options the user has already picked (the desktop's selected set).
  /// Pass a thunk when it can change after mount.
  selected: string[] | (() => string[]);
  /// A pre-read custom answer, used when no composer input entity is passed
  /// (the desktop read it from the `custom_input` entity instead). Pass a
  /// thunk when it can change after mount.
  customAnswer?: string | (() => string);
  /// Optional composer input rendered in the custom-answer slot; its trimmed
  /// content wins over `customAnswer` (the desktop read the entity's text).
  customInput?: JSX.Element;
  isLast: boolean | (() => boolean);
  submitting: boolean | (() => boolean);
  onAnswer: (answer: string | string[]) => void;
  onSelect?: (option: string) => void;
  onSkip?: () => void;
}) {
  // `selected`, `submitting`, `customAnswer` and `isLast` change while the
  // card is mounted; the component runs once, so reads go through thunks.
  const selected = () =>
    typeof props.selected === "function" ? props.selected() : props.selected;
  const customAnswer = () =>
    (typeof props.customAnswer === "function"
      ? props.customAnswer()
      : (props.customAnswer ?? ""))?.trim() ?? "";
  const submitting = () =>
    typeof props.submitting === "function" ? props.submitting() : props.submitting;
  const isLast = () =>
    typeof props.isLast === "function" ? props.isLast() : props.isLast;
  const isMulti = props.request.isMultiSelect ?? false;
  const options = props.request.options ?? [];
  const hasAnswer = () => customAnswer().length > 0 || selected().length > 0;
  const submitLabel = () => (isLast() ? "Submit" : "Continue");

  const answerValue = (): string | string[] => {
    if (customAnswer().length > 0) return customAnswer();
    if (isMulti) return selected();
    return selected()[0] ?? "";
  };

  return (
    <View class="w-full max-w-[728px] flex-col gap-[10px] rounded-xl border border-primary/35 bg-card p-[14px] shadow-lg">
      {/* Question header. `min-w-0` + `flex-1` on the text container so long
          questions wrap instead of bleeding past the card edge; the icon is
          shrink-proof so the row layout stays intact. */}
      <View class="min-w-0 flex-row items-start gap-2">
        <View class="mt-[2px] flex-none">
          <Icon name="Bot" size={14} color={token("primary")} />
        </View>
        <View class="min-w-0 flex-1">
          <MarkdownBlocks source={props.request.question} metrics={METRICS.BODY} />
        </View>
      </View>
      {/* Options */}
      {options.length > 0 ? (
        <View class="flex-col gap-1 pl-6">
          {options.map((opt) => {
            const isChecked = selected().includes(opt);
            return (
              <View
                class={
                  "flex-row items-center gap-2 rounded-md border px-[10px] py-[7px] " +
                  (isChecked
                    ? "border-primary/40 bg-primary/10"
                    : "border-border bg-muted")
                }
                role="button"
                accessibilityLabel={opt}
                onPress={() => props.onSelect?.(opt)}
              >
                <View
                  class={
                    "h-[14px] w-[14px] flex-none flex-row items-center justify-center border " +
                    (isMulti ? "rounded-[3px]" : "rounded-full") +
                    (isChecked
                      ? " border-primary bg-primary"
                      : " border-muted-foreground")
                  }
                >
                  {isChecked ? (
                    <Icon name="Check" size={9} color={token("primary-foreground")} />
                  ) : null}
                </View>
                <View class="min-w-0 flex-1">
                  <MarkdownBlocks source={opt} metrics={METRICS.BODY} />
                </View>
              </View>
            );
          })}
        </View>
      ) : null}
      {/* A custom answer is always available, including when options are
          present. This matches the desktop question panel and lets users
          answer values the model did not pre-populate. */}
      {props.customInput ? (
        <View class="h-8 rounded-md border border-border bg-muted px-2 pl-6">
          {props.customInput}
        </View>
      ) : null}
      {/* Footer actions */}
      <View class="flex-row items-center gap-2 pl-6 pt-1">
        <View
          class={
            "h-7 flex-row items-center justify-center rounded-md px-[14px] " +
            (hasAnswer() && !submitting() ? "bg-primary" : "bg-muted")
          }
          role="button"
          accessibilityLabel={submitLabel()}
          onPress={
            hasAnswer() && !submitting()
              ? () => props.onAnswer(answerValue())
              : undefined
          }
        >
          <Text
            class={
              "text-[12px] font-semibold " +
              (hasAnswer() && !submitting()
                ? "text-primary-foreground"
                : "text-muted-foreground")
            }
          >
            {submitLabel()}
          </Text>
        </View>
        {props.onSkip ? (
          <View
            class="flex-row items-center justify-center rounded-md px-3 py-[6px] hover:bg-accent"
            role="button"
            accessibilityLabel="Skip"
            onPress={props.onSkip}
          >
            <Text class="text-[12px] text-muted-foreground">Skip</Text>
          </View>
        ) : null}
      </View>
    </View>
  );
}

function prettyJson(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? "";
  } catch {
    return String(value);
  }
}
