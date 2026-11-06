// The user message bubble, ported from console-ui's `chat/message_bubble.rs`
// (the `UserMessageBubble` half). It pins image attachments and
// browser-annotation tiles above a right-aligned card, folds long prompts
// behind a Show more toggle, and paints staged file comments as chips.
//
// Deviations from the Rust source: the GPUI transcript-selection machinery
// (selectable runs, the selection registry, row keys) has no nsh
// counterpart, so the bubble renders plain text and the Copy/Cite selection
// popup is dropped; and nsh exposes no group hover, so the footer renders
// quietly but always.

import { Image, Text, View } from "nsh";
import { CopyButton } from "../common/copy_button";
import { Icon, type IconName } from "../common/icons";
import { baseName, mentionLabel } from "../utils/mentions";
import { fileTypeIconName } from "./change_chips";
import {
  messageTextWithMentions,
  splitFileComments,
  type FileCommentRef,
} from "../utils/mentions";
import { formatMessageTime } from "../utils/time";

export interface ImageAttachment {
  data: string;
  mimeType: string;
}

/// console-core's `BrowserElementAnnotation`.
export interface BrowserElementAnnotation {
  selector: string;
  componentName?: string | null;
  sourceLocation?: string | null;
  screenshotBase64?: string | null;
}


export function dataUri(data: string, mimeType: string): string {
  return `data:${mimeType};base64,${data}`;
}

/// The label an annotation tile shows when it has no screenshot.
function annotationLabel(ann: BrowserElementAnnotation): string {
  if (ann.componentName && ann.sourceLocation) {
    return `<${ann.componentName} />  •  ${ann.sourceLocation}`;
  }
  if (ann.componentName) return `<${ann.componentName} />`;
  if (ann.sourceLocation) return ann.sourceLocation;
  return ann.selector;
}

/// A preview tile: one image (screenshot or attachment) with a press handler
/// that opens the app's image preview modal.
function ImageTile(props: {
  id: string;
  uri: string | undefined;
  fallbackLabel?: string;
  onPress?: () => void;
}) {
  return (
    <View
      class="h-[80px] w-[80px] flex-none overflow-hidden rounded-[9px] border border-border bg-black"
      role="button"
      accessibilityLabel={props.id}
      onPress={props.onPress}
    >
      {props.uri != null ? (
        <Image src={props.uri} style={{ width: 80, height: 80 }} />
      ) : (
        <View class="flex h-full w-full flex-col bg-muted p-1.5">
          <Text class="text-[10.5px] text-muted-foreground">
            {props.fallbackLabel}
          </Text>
        </View>
      )}
    </View>
  );
}

/// One staged file comment painted as a chip above the bubble's text, like
/// a mention (`file_mention_chip`).
function FileCommentChip(props: { comment: FileCommentRef }) {
  const comment = props.comment;
  const name = baseName(comment.path);
  const lines = comment.lines.replace("–", "-");
  return (
    <View class="flex-row items-center gap-1 rounded-md border border-border bg-muted px-1.5 py-0.5">
      <Icon
        name={fileTypeIconName(comment.path) as IconName}
        size={13}
        mono={false}
      />
      <Text class="text-[12.5px] text-muted-foreground">{`${name} +${lines}`}</Text>
    </View>
  );
}

// ─── User message ────────────────────────────────────────────────────────────

/// Fold threshold for user message text: parity with mobile's 4-line fold,
/// scaled up since desktop lines hold ~2x the characters of a phone line.
const USER_FOLDED_LINES = 8;
/// 8 lines at the bubble's 20px line height.
const USER_FOLDED_MAX_HEIGHT = 8.0 * 20.0;
/// Conservative characters-per-line estimate for 14px text in the ~512px
/// message column (the bubble caps at 540px with 14px side padding).
const USER_FOLD_CHARS_PER_LINE = 64;

/// Rough wrapped-line count for fold decisions: hard breaks plus length-based
/// wrapping per segment (`estimated_text_lines`).
export function estimatedTextLines(text: string): number {
  return text
    .split("\n")
    .map((line) =>
      Math.max(1, Math.ceil(line.length / USER_FOLD_CHARS_PER_LINE)),
    )
    .reduce((sum, wrapped) => sum + wrapped, 0);
}

/// The user's prompt: attachments and annotation tiles above, the bubble
/// card (with folded text and comment chips), then a footer of timestamp and
/// copy. Long prompts fold to 8 lines with a Show more/less toggle inside
/// the card, mirroring mobile's 4-line fold.
export function UserMessageBubble(props: {
  content: string | (() => string);
  attachments?: ImageAttachment[];
  annotations?: BrowserElementAnnotation[];
  contextFiles?: string[];
  createdAt?: number | null;
  expanded?: boolean | (() => boolean);
  onToggleExpand?: () => void;
  onPreviewImage?: (uri: string) => void;
}) {
  const content = () =>
    typeof props.content === "function" ? props.content() : props.content;
  const expanded = () =>
    typeof props.expanded === "function" ? props.expanded() : (props.expanded ?? false);

  // Staged file comments ride the prompt as tagged blocks; show them as
  // chips like mentions instead of dumping the raw markup in the bubble.
  const [bodyText, fileComments] = splitFileComments(content());
  const [messageText, mentions] = messageTextWithMentions(
    bodyText,
    props.contextFiles ?? [],
  );
  const hasContent = messageText.length > 0;
  const foldable = hasContent && estimatedTextLines(messageText) > USER_FOLDED_LINES;
  const folded = () => foldable && !expanded();

  // The mention ranges index into the rendered string; paint those segments
  // as inline pills and the rest plain (the desktop's
  // `InlineFileMentionText`).
  const segments = (): { text: string; path?: string }[] => {
    const sorted = [...mentions].sort((a, b) => a.range.start - b.range.start);
    const parts: { text: string; path?: string }[] = [];
    let cursor = 0;
    for (const mention of sorted) {
      if (mention.range.start < cursor) continue;
      if (mention.range.start > cursor) {
        parts.push({ text: messageText.slice(cursor, mention.range.start) });
      }
      parts.push({
        text: messageText.slice(mention.range.start, mention.range.end),
        path: mention.path,
      });
      cursor = mention.range.end;
    }
    if (cursor < messageText.length) {
      parts.push({ text: messageText.slice(cursor) });
    }
    return parts;
  };

  // Folded prompts render only their visible lines — the desktop clips at
  // the box, but a clipped Text still exposes every line to reading and
  // tests.
  const foldedText = (text: string) => {
    if (!folded()) return text;
    const lines = text.split("\n");
    return lines.slice(0, USER_FOLDED_LINES).join("\n");
  };

  const messageTextElement = (
    <Text>
      {segments().map((segment) =>
        segment.path != null ? (
          <Text class="rounded bg-muted px-[2.5px] text-primary">
            {foldedText(segment.text)}
          </Text>
        ) : (
          <Text>{foldedText(segment.text)}</Text>
        ),
      )}
    </Text>
  );

  return (
    <View class="w-full flex-col items-end gap-[3px]">
      {/* Message: image(s) pinned to the top, user bubble card below. */}
      <View class="max-w-[540px] flex-col items-end gap-2">
        {(props.attachments?.length ?? 0) > 0 ? (
          <View class="flex-row flex-wrap justify-end gap-2">
            {props.attachments!.map((attachment) => (
              <ImageTile
                id="user attachment"
                uri={dataUri(attachment.data, attachment.mimeType)}
                onPress={() =>
                  props.onPreviewImage?.(
                    dataUri(attachment.data, attachment.mimeType),
                  )
                }
              />
            ))}
          </View>
        ) : null}
        {(props.annotations?.length ?? 0) > 0 ? (
          <View class="flex-row flex-wrap justify-end gap-2">
            {props.annotations!.map((ann, index) => {
              const thumb =
                ann.screenshotBase64 && ann.screenshotBase64.length > 0
                  ? dataUri(ann.screenshotBase64, "image/png")
                  : undefined;
              return (
                <ImageTile
                  id={`user annotation ${index}`}
                  uri={thumb}
                  fallbackLabel={annotationLabel(ann)}
                  onPress={thumb != null ? () => props.onPreviewImage?.(thumb) : undefined}
                />
              );
            })}
          </View>
        ) : null}
        {hasContent ? (
          <View class="max-w-[540px] flex-col rounded-xl border border-border bg-muted px-[14px] py-[9px]">
            <View
              class="flex-col"
              style={
                folded()
                  ? { maxHeight: USER_FOLDED_MAX_HEIGHT, overflow: "hidden" }
                  : undefined
              }
            >
              {fileComments.length > 0 ? (
                <View class="mb-1.5 flex-row flex-wrap justify-end gap-1.5">
                  {fileComments.map((comment) => (
                    <FileCommentChip comment={comment} />
                  ))}
                </View>
              ) : null}
              {messageTextElement}
            </View>
            {foldable ? (
              <View
                class="pt-1"
                role="button"
                accessibilityLabel="Toggle prompt fold"
                onPress={() => props.onToggleExpand?.()}
              >
                <Text class="text-[12px] text-primary">
                  {expanded() ? "Show less" : "Show more"}
                </Text>
              </View>
            ) : null}
          </View>
        ) : null}
      </View>
      {/* Footer: timestamp + copy (the desktop shows these on hover only). */}
      <View class="h-[27px] flex-row items-center gap-1">
        {(() => {
          const timestamp = formatMessageTime(props.createdAt);
          return timestamp != null ? (
            <Text class="px-1 text-[11.5px] text-muted-foreground">
              {timestamp}
            </Text>
          ) : null;
        })()}
        <CopyButton id="copy-user-message" content={content()} />
      </View>
    </View>
  );
}
