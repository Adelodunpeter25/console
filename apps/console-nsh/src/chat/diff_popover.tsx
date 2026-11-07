// The diff popover for the per-turn changed-file chips, ported from
// console-ui's `chat/diff_popover.rs`: a card above a chip showing that
// file's recorded diff, or a short "Binary file …" note for binary files
// with no stored diff; other files without a diff get no popover.
//
// The desktop opens the card on hover with show/hide delays; the card here
// is nsh's `Popover`, opened by pressing the chip (the hover equivalent —
// nsh's `HoverCard` — is available and can replace this), with the preview
// rendered through the shared `DiffView` (gutter, change bars), capped,
// full diff one press away.

import { Text, View } from "nsh";
import type { JSX } from "nsh";
import { Popover } from "nsh/base";
import { DiffView, parseUnifiedDiff } from "./diff_view";
import type { TurnChange } from "./change_chips";

/// A hover (here: press) preview only builds this many diff lines. Past it
/// the card says how much was left out; the full diff is one press away.
/// 200, not the desktop's 400: each DiffView row is a small tree (gutter,
/// change bar, text), and the popup renders its first frame headless — 200
/// rows stay inside one frame's budget.
const PREVIEW_MAX_LINES = 200;
/// Longer previews scroll inside this height (`CARD_DIFF_MAX_HEIGHT`).
const CARD_DIFF_MAX_HEIGHT = 260;

/// File kinds the desktop's `file_kind_for_path` reports as binary (raster
/// images and blocked kinds) — the extensions a change row plausibly carries.
const BINARY_EXTENSIONS = new Set([
  "png", "jpg", "jpeg", "gif", "webp", "bmp", "ico", "tiff", "avif", "heic",
  "zip", "tar", "gz", "tgz", "rar", "7z", "exe", "dll", "dylib", "so", "dmg",
  "pdf", "mp4", "mov", "mp3", "wav", "woff", "woff2", "ttf", "otf",
]);

function isBinaryPath(path: string): boolean {
  const dot = path.lastIndexOf(".");
  if (dot <= 0) return false;
  return BINARY_EXTENSIONS.has(path.slice(dot + 1).toLowerCase());
}

/// What a card shows (`Body`): the diff text, or a note.
type Body = { kind: "diff"; text: string } | { kind: "note"; note: string };

function bodyFor(change: TurnChange): Body | null {
  if (change.diffText != null && change.diffText.trim().length > 0) {
    return { kind: "diff", text: change.diffText };
  }
  if (isBinaryPath(change.path)) {
    const what =
      change.status === "added"
        ? "added"
        : change.status === "deleted"
          ? "deleted"
          : "modified";
    return { kind: "note", note: `Binary file ${what}` };
  }
  return null;
}

function DiffCardBody(props: {
  change: TurnChange;
  onOpen?: (path: string, diffText: string | null) => void;
}) {
  // The body depends only on this chip's static change row, so it computes
  // once per mount.
  const body = bodyFor(props.change);
  if (body == null) return null;
  if (body.kind === "note") {
    return (
      <Text class="px-2 py-1.5 text-[12px] text-muted-foreground">
        {body.note}
      </Text>
    );
  }
  const parsed = parseUnifiedDiff(body.text);
  const preview = parsed.slice(0, PREVIEW_MAX_LINES);
  const leftOut = parsed.length - preview.length;
  return (
    <View class="w-full flex-col gap-1">
      <View class="overflow-hidden rounded-md border border-border bg-card p-1">
        <DiffView lines={preview} maxHeight={CARD_DIFF_MAX_HEIGHT} />
      </View>
      {leftOut > 0 ? (
        <View
          class="rounded px-2 pt-[5px] pb-0.5 hover:bg-accent"
          role="button"
          accessibilityLabel="Open full diff"
          onPress={() =>
            props.onOpen?.(props.change.path, props.change.diffText)
          }
        >
          <Text class="text-[11px] text-muted-foreground">
            {`Showing the first ${preview.length} of ${parsed.length} lines — click the file to open the full diff`}
          </Text>
        </View>
      ) : null}
    </View>
  );
}

/// Give a chip its diff popover (`with_diff_popover`): `trigger` is the chip.
/// Returns the chip unchanged when the file has nothing to show.
export function WithDiffPopover(props: {
  change: TurnChange;
  trigger: JSX.Element;
  onOpen?: (path: string, diffText: string | null) => void;
}): JSX.Element {
  const body = bodyFor(props.change);
  if (body == null) return props.trigger;
  return (
    <Popover
      placement="top"
      align="start"
      offset={6}
      popupClass="rounded-[10px] border border-border bg-popover p-1 shadow-md"
      popupStyle={{ width: 560, maxWidth: 380 }}
      trigger={props.trigger}
    >
      <DiffCardBody change={props.change} onOpen={props.onOpen} />
    </Popover>
  );
}
