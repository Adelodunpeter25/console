// The assistant message bubble, ported from console-ui's
// `chat/message_bubble.rs` (the `AssistantMessageBubble` half). It walks its
// content parts top to bottom (thinking disclosures, markdown text, inline
// images — tool calls render in the run's grouped activity, not here), then
// this turn's changed-file chips and a footer of timestamp, throughput,
// fork and copy.
//
// Deviations: the desktop's markdown-view/presentation caches are dropped —
// nsh reactivity re-parses only when a part's text changes — and the
// hover-only footer renders always (no group hover in nsh).

import { Image, Text, View, token } from "nsh";
import { Icon } from "../common/icons";
import { CopyButton } from "../common/copy_button";
import { TurnChangeChips, type TurnChanges } from "./change_chips";
import { ThinkingBlock } from "./thinking_block";
import { MarkdownBlocks, METRICS } from "./markdown/render";
import { dataUri, type ImageAttachment } from "./message_user";
import type { ToolCall } from "./toolcalls/model";
import { formatMessageTime, formatTokensPerSecond } from "../utils/time";

export type AssistantContentPart =
  | { type: "thinking"; text: string }
  | { type: "text"; text: string }
  | { type: "toolCall"; call: ToolCall }
  | { type: "image"; data: string; mimeType: string };

/// A file mention painted inside a message's text: the `range` covers the

// ─── Assistant message ───────────────────────────────────────────────────────

/// The assistant's reply: its content parts top to bottom (thinking
/// disclosures, markdown text, inline images — tool calls are rendered in
/// the run's grouped activity, not here), then this turn's changed-file
/// chips and a footer of timestamp, throughput, fork and copy.
export function AssistantMessageBubble(props: {
  contentParts: AssistantContentPart[] | (() => AssistantContentPart[]);
  isStreaming?: boolean | (() => boolean);
  createdAt?: number | null;
  /// Server-computed output throughput for this turn, already gated there.
  tokPerSec?: number | null;
  contentForCopy?: string | (() => string);
  forkMessageId?: string | null;
  onFork?: (messageId: string) => void;
  thinkingExpanded?: Set<string> | (() => Set<string>);
  onThinkingToggle?: (thinkingId: string, expanded: boolean) => void;
  fileChanges?: TurnChanges | null;
  changesExpanded?: boolean | (() => boolean);
  onToggleChanges?: () => void;
  onOpenChange?: (path: string, diffText: string | null) => void;
  onPreviewImage?: (uri: string) => void;
}) {
  const parts = () =>
    typeof props.contentParts === "function"
      ? props.contentParts()
      : props.contentParts;
  const isStreaming = () =>
    typeof props.isStreaming === "function"
      ? props.isStreaming()
      : (props.isStreaming ?? false);
  const contentForCopy = () => {
    if (props.contentForCopy != null) {
      return typeof props.contentForCopy === "function"
        ? props.contentForCopy()
        : props.contentForCopy;
    }
    return parts()
      .filter((part) => part.type === "text" || part.type === "thinking")
      .map((part) => part.text)
      .join("\n\n");
  };
  const thinkingExpanded = () =>
    typeof props.thinkingExpanded === "function"
      ? props.thinkingExpanded()
      : (props.thinkingExpanded ?? new Set<string>());

  // Fork only applies to a settled message.
  const forkable = () => props.forkMessageId != null && !isStreaming();

  return (
    <View class="w-full flex-col gap-1.5">
      {parts().map((part, index) => {
        if (part.type === "thinking") {
          const collapsed = !thinkingExpanded().has(`part-${index}`);
          return (
            <ThinkingBlock
              id={`part-${index}`}
              text={part.text}
              collapsed={collapsed}
              onToggle={(expanded) =>
                props.onThinkingToggle?.(`part-${index}`, expanded)
              }
            />
          );
        }
        if (part.type === "text") {
          // Tool calls are rendered in the run's grouped activity, not
          // inline in the assistant bubble. Keeping this branch empty also
          // prevents the same call from appearing twice while a streamed
          // turn is replaced by its persisted form.
          return (
            <MarkdownBlocks source={part.text} metrics={METRICS.BODY} />
          );
        }
        if (part.type === "toolCall") {
          return null;
        }
        // Inline image: decode the payload into a data URI; skip the part
        // when the format is not supported rather than rendering a blank tile.
        const supported = ["image/png", "image/jpeg", "image/gif", "image/webp"];
        if (!supported.includes(part.mimeType)) return null;
        const uri = dataUri(part.data, part.mimeType);
        return (
          <View
            class="max-w-[400px] overflow-hidden rounded-lg border border-border"
            role="button"
            accessibilityLabel={`assistant image ${index}`}
            onPress={() => props.onPreviewImage?.(uri)}
          >
            <Image src={uri} style={{ width: 400, height: 300 }} />
          </View>
        );
      })}
      {/* Footer: this turn's changed-file chips (always visible), then
          time + throughput + fork + copy (hover only on the desktop). */}
      <View class="flex-col gap-1">
        {props.fileChanges != null && props.fileChanges.files.length > 0 ? (
          <TurnChangeChips
            turn={props.fileChanges}
            expanded={props.changesExpanded ?? false}
            onOpen={props.onOpenChange}
            onToggle={props.onToggleChanges}
          />
        ) : null}
        <View class="h-[27px] flex-row items-center gap-1">
          {(() => {
            const timestamp = formatMessageTime(props.createdAt);
            return timestamp != null ? (
              <Text class="px-1 text-[11.5px] text-muted-foreground">
                {timestamp}
              </Text>
            ) : null;
          })()}
          {(() => {
            const throughput = formatTokensPerSecond(props.tokPerSec);
            return throughput != null ? (
              <Text class="px-1 text-[11.5px] text-muted-foreground">
                {throughput}
              </Text>
            ) : null;
          })()}
          {forkable() ? (
            <View
              class="h-[27px] w-[27px] flex-row items-center justify-center rounded-lg hover:bg-accent"
              role="button"
              accessibilityLabel={`Fork from message ${props.forkMessageId}`}
              onPress={() => props.onFork?.(props.forkMessageId!)}
            >

              <Icon name="Fork" size={14} color={token("muted-foreground")} />
            </View>
          ) : null}
        </View>
      </View>
    </View>
  );
}
