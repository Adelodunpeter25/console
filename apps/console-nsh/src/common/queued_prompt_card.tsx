// The queued next-turn prompt card shown above the composer while a run is
// active, ported from console-ui's `common/queued_prompt_card.rs`: a compact
// horizontal pill with inline Edit / Delete / Steer actions, stacked for
// multiple queued prompts.

import { Image, Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { CenteredStripe } from "./stripe";
import { Icon } from "./icons";
import type { ComposerAttachment } from "./composer";

/// console-core's `QueuedPrompt` wire shape (annotations are component
/// selectors the session queued with the prompt).
export interface QueuedPrompt {
  id: string;
  prompt: string;
  attachments: ComposerAttachment[];
  annotations: { selector: string; componentName?: string }[];
}

/// Formats prompt text for compact preview: if it contains multiple lines,
/// takes the first line and appends `...`. Long single lines are also
/// truncated (70 chars multi-line, 90 single-line).
export function formatPromptPreview(raw: string): string {
  const trimmed = raw.trim();
  const lines = trimmed
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
  const firstLine = lines[0];
  if (firstLine == null) return "";
  const hasMoreLines = lines.length > 1;

  if (hasMoreLines) {
    return firstLine.length > 70
      ? `${firstLine.slice(0, 70).trimEnd()}...`
      : `${firstLine}...`;
  }
  return firstLine.length > 90
    ? `${firstLine.slice(0, 90).trimEnd()}...`
    : firstLine;
}

function IconButton(props: {
  label: string;
  icon: "Pencil" | "TrashBinMinimalistic" | "ArrowUp";
  destructive?: boolean;
  onPress?: () => void;
}) {
  return (
    <View
      class={
        "h-6 w-6 flex-row items-center justify-center rounded " +
        (props.destructive
          ? "hover:bg-destructive/20 active:bg-destructive/20"
          : "hover:bg-accent active:bg-accent")
      }
      role="button"
      accessibilityLabel={props.label}
      onPress={props.onPress}
    >
      <Icon
        name={props.icon}
        size={12}
        color={
          props.destructive
            ? token("destructive")
            : token("muted-foreground")
        }
      />
    </View>
  );
}

/// One queued prompt's pill. Attachments show their first image as a
/// thumbnail (with a "+N" for the rest) or a paperclip; annotations show
/// their component name pill.
export function QueuedPromptRow(props: {
  prompt: QueuedPrompt;
  onEdit?: () => void;
  onDelete?: () => void;
  onSteer?: () => void;
}) {
  const prompt = props.prompt;
  const firstImage = prompt.attachments[0]?.url;
  const extraAttachments = Math.max(0, prompt.attachments.length - 1);
  const showAttachmentIcon =
    prompt.attachments.length > 0 && firstImage == null;
  const firstAnnotation = prompt.annotations[0];

  const left: JSX.Element[] = [];
  if (firstImage != null) {
    left.push(
      <View class="flex-none flex-row items-center gap-1">
        <View class="h-[22px] w-[22px] flex-none overflow-hidden rounded bg-black">
          <Image src={firstImage} style={{ width: 22, height: 22 }} />
        </View>
        {extraAttachments > 0 ? (
          <Text class="text-[11px] text-muted-foreground">
            +{extraAttachments}
          </Text>
        ) : null}
      </View>,
    );
  }
  if (firstAnnotation) {
    left.push(
      <View
        class="flex-none rounded-full border border-border px-1.5 py-0.5"
      >
        <Text class="text-[10.5px] text-muted-foreground">
          {firstAnnotation.componentName ?? firstAnnotation.selector}
        </Text>
      </View>,
    );
  }
  left.push(
    <Text class="min-w-0 flex-1 text-[13px] leading-[18px] text-foreground">
      {formatPromptPreview(prompt.prompt)}
    </Text>,
  );
  if (showAttachmentIcon) {
    left.push(
      <View class="flex-none">
        <Icon name="Paperclip" size={11} color={token("muted-foreground")} />
      </View>,
    );
  }

  return (
    <View class="w-full max-w-[728px] flex-row items-center justify-between gap-2.5 rounded-lg border border-border bg-card px-3 py-2 shadow-sm">
      <View class="min-w-0 flex-1 flex-row items-center gap-2">{left}</View>
      <View class="flex-none flex-row items-center gap-0.5">
        <IconButton
          label={`Edit queued prompt ${prompt.id}`}
          icon="Pencil"
          onPress={props.onEdit}
        />
        <IconButton
          label={`Delete queued prompt ${prompt.id}`}
          icon="TrashBinMinimalistic"
          destructive
          onPress={props.onDelete}
        />
        <IconButton
          label={`Send queued prompt ${prompt.id} now`}
          icon="ArrowUp"
          onPress={props.onSteer}
        />
      </View>
    </View>
  );
}

export function QueuedPromptCard(props: {
  prompt: QueuedPrompt;
  onEdit?: () => void;
  onDelete?: () => void;
  onSteer?: () => void;
}) {
  return (
    <CenteredStripe paddingY={6}>
      <QueuedPromptRow
        prompt={props.prompt}
        onEdit={props.onEdit}
        onDelete={props.onDelete}
        onSteer={props.onSteer}
      />
    </CenteredStripe>
  );
}

/// The stack of queued prompts, one pill per prompt, callbacks keyed by id.
export function QueuedPromptsStack(props: {
  prompts: QueuedPrompt[] | (() => QueuedPrompt[]);
  onEdit?: (id: string) => void;
  onDelete?: (id: string) => void;
  onSteer?: (id: string) => void;
}) {
  const prompts = () =>
    typeof props.prompts === "function" ? props.prompts() : props.prompts;

  return (
    <CenteredStripe paddingY={6}>
      <View class="w-full max-w-[728px] flex-col gap-1.5">
        {prompts().map((prompt) => (
          <QueuedPromptRow
            prompt={prompt}
            onEdit={props.onEdit ? () => props.onEdit!(prompt.id) : undefined}
            onDelete={
              props.onDelete ? () => props.onDelete!(prompt.id) : undefined
            }
            onSteer={
              props.onSteer ? () => props.onSteer!(prompt.id) : undefined
            }
          />
        ))}
      </View>
    </CenteredStripe>
  );
}
