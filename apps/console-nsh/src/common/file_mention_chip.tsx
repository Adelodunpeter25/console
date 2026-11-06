// The inline file-mention chip, ported from console-ui's
// `common/file_mention_chip.rs`: a file-type icon plus the friendly label in
// an accent-tinted pill. The composer and the message bubble share it so the
// two stay visually identical; the desktop's styled-text painting machinery
// (`InlineFileMentionText`) is GPUI-specific and has no port — nsh nests
// Texts inside one paragraph instead.

import { Text, View, token } from "nsh";
import { Icon, type IconName } from "./icons";
import { fileTypeIconName } from "../chat/change_chips";

/// Icon size for the chip, shared by the composer and the bubble.
export const FILE_MENTION_ICON_SIZE = 11;

/// Corner radius for the pill.
export const FILE_MENTION_RADIUS = 4;

/// The chat- and skill-path prefixes (`console_shared::composer::mentions`).
export const CHAT_PATH_PREFIX = "chat:";
export const SKILL_PATH_PREFIX = "skill:";

/// The icon a mention chip paints: a file-type icon for files, one generic
/// icon for skills and chats.
export function mentionIconName(path: string): IconName {
  if (path.startsWith(CHAT_PATH_PREFIX)) return "ChatRoundLine";
  if (path.startsWith(SKILL_PATH_PREFIX)) return "Sparkle";
  return fileTypeIconName(path) as IconName;
}

/// The chip: file-type icon + filename label in an accent wash. Colors come
/// from tokens (the desktop tints `accent` at fixed opacities, which text
/// colors in nsh cannot carry — the wash uses the theme's muted background
/// with an accent border and label at the same strength).
export function FileMentionChip(props: {
  /// The mention's path (a real file path, `chat:slug`, or `skill:name`).
  path: string;
  /// The painted label: the file's basename, `$slug`, or `/skill`.
  label: string;
}) {
  return (
    <View
      class="flex-row items-center gap-1 rounded border border-primary/30 bg-muted px-1.5 py-[1px]"
      style={{ borderRadius: FILE_MENTION_RADIUS }}
    >
      <Icon
        name={mentionIconName(props.path)}
        size={FILE_MENTION_ICON_SIZE}
        color={token("primary")}
      />
      <Text class="text-primary" style={{ fontSize: 12.5 }}>
        {props.label}
      </Text>
    </View>
  );
}
