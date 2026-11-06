// The chat composer, ported from console-ui's `common/composer_view.rs`
// (first cut). Full-card variant: staged chips row, attachment thumbnails,
// a multiline input, and the footer of model / approval / thinking chips
// with attach and send. The popovers behind the model and approval chips
// (the dropdowns) and the floating compact variant land with the
// primitives pass; until then the chips press but don't open menus.
//
// NSH renders components once and tracks prop reads in JSX, so all values
// derived from props are read lazily (`()` thunks): normalizing a prop into
// a body-level const would freeze it at mount.

import { Image, Text, View, token } from "nsh";
import { Input } from "nsh/base";
import { Icon, type IconName } from "./icons";

/// A staged file comment chip above the composer's text field
/// (`composer_view.rs` `ComposerComment`).
export type ComposerComment = {
  id: number;
  path: string;
  label: string;
  onEdit?: () => void;
  onRemove?: () => void;
};

/// A staged browser element annotation chip (`ComposerAnnotation`).
export type ComposerAnnotation = {
  id: string;
  label: string;
  tooltip?: string;
  onRemove?: () => void;
};

/// A staged image attachment (`ImageAttachment` of console-core; the app
/// shows it by URL until the attachment service moves over).
export type ComposerAttachment = {
  index: number;
  url: string;
};

/// The run state the footer reflects (`ComposerRunState`): whether a chat
/// run is live (send becomes stop) or the composer is ready.
export type ComposerRunState = "ready" | "running";

/// The approval modes of the desktop (`console-core`'s `ApprovalMode`).
export const APPROVAL_MODES = [
  "AlwaysAsk",
  "AcceptEdits",
  "PlanMode",
  "FullAccess",
] as const;
export type ApprovalMode = (typeof APPROVAL_MODES)[number];

export function approvalModeLabel(mode: ApprovalMode): string {
  switch (mode) {
    case "AlwaysAsk":
      return "Always Ask";
    case "AcceptEdits":
      return "Accept Edits";
    case "PlanMode":
      return "Plan Mode";
    case "FullAccess":
      return "Full Access";
  }
}

export function approvalModeIcon(mode: ApprovalMode): IconName {
  // The desktop's `ApprovalModeIconExt::icon`.
  switch (mode) {
    case "AlwaysAsk":
      return "TriangleAlert";
    case "AcceptEdits":
      return "Pencil";
    case "PlanMode":
      return "List";
    case "FullAccess":
      return "Zap";
  }
}

/// The thinking levels of the desktop (`console-core`'s `ThinkingLevel`).
export const THINKING_LEVELS = [
  "None",
  "Minimal",
  "Low",
  "Medium",
  "High",
  "XHigh",
  "Max",
] as const;
export type ThinkingLevel = (typeof THINKING_LEVELS)[number];

export function thinkingLevelLabel(level: ThinkingLevel): string {
  return level === "None" ? "No thinking" : `Thinking: ${level.toLowerCase()}`;
}

export function Composer(props: {
  value: string;
  onChange: (text: string) => void;
  onSend: () => void;
  runState?: ComposerRunState;
  approvalMode?: ApprovalMode;
  onApprovalMode?: (mode: ApprovalMode) => void;
  modelLabel?: string | null;
  thinkingLevel?: ThinkingLevel;
  onCycleThinking?: () => void;
  attachments?: ComposerAttachment[];
  onRemoveAttachment?: (index: number) => void;
  comments?: ComposerComment[];
  annotations?: ComposerAnnotation[];
  placeholder?: string;
  testID?: string;
}) {
  const runState = () => props.runState ?? "ready";
  const isRunning = () => runState() === "running";
  const attachments = () => props.attachments ?? [];
  const comments = () => props.comments ?? [];
  const annotations = () => props.annotations ?? [];
  const hasDraft = () =>
    props.value.trim() !== "" ||
    attachments().length > 0 ||
    comments().length > 0 ||
    annotations().length > 0;
  const approvalMode = () => props.approvalMode ?? "AlwaysAsk";
  const thinkingLevel = () => props.thinkingLevel ?? "None";

  const send = () => {
    if (hasDraft() && !isRunning()) props.onSend();
  };

  return (
    <View
      class="flex flex-col rounded-2xl border border-border bg-card"
      testID={props.testID}
    >
      {comments().length > 0 ? (
        <View class="flex flex-row flex-wrap gap-1.5 px-3 pb-1.5 pt-3">
          {comments().map((comment) => (
            <View class="flex-row items-center gap-1 rounded-full border border-border bg-muted px-2.5 py-1">
              <Text class="text-[11px] text-foreground">
                {comment.label}
              </Text>
              {comment.onEdit ? (
                <View onPress={comment.onEdit} role="button">
                  <Icon name="Pen" size={11} color={token("muted-foreground")} />
                </View>
              ) : null}
              {comment.onRemove ? (
                <View onPress={comment.onRemove} role="button">
                  <Icon name="X" size={11} color={token("muted-foreground")} />
                </View>
              ) : null}
            </View>
          ))}
        </View>
      ) : null}

      {annotations().length > 0 ? (
        <View class="flex flex-row flex-wrap gap-1.5 px-3 pb-1.5 pt-3">
          {annotations().map((annotation) => (
            <View class="flex-row items-center gap-1 rounded-full border border-border bg-muted px-2 py-1">
              <Text class="text-[11px] text-muted-foreground">
                {annotation.label}
              </Text>
              {annotation.onRemove ? (
                <View onPress={annotation.onRemove} role="button">
                  <Icon name="X" size={11} color={token("muted-foreground")} />
                </View>
              ) : null}
            </View>
          ))}
        </View>
      ) : null}

      {attachments().length > 0 ? (
        <View class="flex-row flex-wrap gap-2 px-3 pb-1.5 pt-3">
          {attachments().map((attachment) => (
            <View class="h-14 w-14 overflow-hidden rounded-lg border border-border">
              <Image src={attachment.url} style={{ width: 56, height: 56 }} />
            </View>
          ))}
        </View>
      ) : null}

      <Input
        multiline
        submitBehavior="submit"
        class="px-3.5 py-3 text-sm text-foreground"
        placeholder={props.placeholder ?? "Ask Console anything…"}
        value={props.value}
        onChange={(e: { value: string }) => props.onChange(e.value)}
        onSubmit={send}
        testID="composer-input"
      />

      <View class="flex-row items-center gap-2 px-3 pb-3">
        {props.modelLabel ? (
          <View
            class="h-6 flex-row items-center gap-1.5 rounded-md border border-border px-2"
            testID="model-chip"
          >
            <Text class="text-[11px] text-foreground">{props.modelLabel}</Text>
            <Icon name="ChevronDown" size={10} color={token("muted-foreground")} />
          </View>
        ) : null}

        <View
          class="h-6 flex-row items-center gap-1.5 rounded-md border border-border px-2"
          onPress={() => {
            if (!props.onApprovalMode) return;
            const modes = APPROVAL_MODES;
            const next = modes[(modes.indexOf(approvalMode()) + 1) % modes.length];
            props.onApprovalMode(next);
          }}
          testID="approval-chip"
          role="button"
        >
          <Icon
            name={approvalModeIcon(approvalMode())}
            size={11}
            color={
              approvalMode() === "FullAccess"
                ? token("destructive")
                : token("muted-foreground")
            }
          />
          <Text class="text-[11px] text-muted-foreground">
            {approvalModeLabel(approvalMode())}
          </Text>
        </View>

        {props.onCycleThinking ? (
          <View
            class="h-6 flex-row items-center gap-1.5 rounded-md border border-border px-2"
            onPress={props.onCycleThinking}
            testID="thinking-chip"
            role="button"
          >
            <Text class="text-[11px] text-muted-foreground">
              {thinkingLevelLabel(thinkingLevel())}
            </Text>
          </View>
        ) : null}

        <View class="flex-1" />

        <View
          class="flex h-8 w-8 items-center justify-center rounded-lg border border-border"
          testID="attach-button"
        >
          <Icon name="Paperclip" size={14} color={token("muted-foreground")} />
        </View>

        <View
          class={
            "flex h-8 w-8 items-center justify-center rounded-lg " +
            (hasDraft() && !isRunning()
              ? "bg-primary"
              : "border border-border opacity-50")
          }
          onPress={send}
          testID="send-button"
          accessibilityLabel={isRunning() ? "Stop" : "Send"}
          role="button"
        >
          <Icon
            name={isRunning() ? "Stop" : "ArrowUp"}
            size={14}
            color={hasDraft() && !isRunning() ? token("primary-foreground") : token("muted-foreground")}
          />
        </View>
      </View>
    </View>
  );
}
