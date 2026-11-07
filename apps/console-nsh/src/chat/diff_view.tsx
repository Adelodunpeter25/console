// The diff preview renderer for the chip popover, modelled on console-ui's
// `viewer/diff_viewer.rs`: line-number gutter, change bars, and colored
// lines at the editor's diff row height. The popover uses this hand-rolled
// view because nsh's native `DiffView` (which the inline tool-call rows use,
// `toolcalls/rows.tsx`) does not lay out inside a popup — a large patch
// renders nothing there, verified headless; in the main tree the native
// view is strictly better (virtualized, syntax-highlighted).

import { Text, View, token } from "nsh";

/// Row height of the diff view (`DIFF_LINE_HEIGHT`).
export const DIFF_LINE_HEIGHT = 20;

/// One diff line (`editor_ui::DiffLine`): its kind, text, and the line
/// numbers it had in the old and new file (null where the line does not
/// exist on that side).
export type DiffLine = {
  kind: "+" | "-" | " ";
  text: string;
  oldNo?: number | null;
  newNo?: number | null;
};

// console-core's `parse_unified_diff` essentials: hunk-header line numbers
// and per-line old/new numbering; file headers and the "\ No newline"
// marker are skipped.
export function parseUnifiedDiff(raw: string): DiffLine[] {
  const lines: DiffLine[] = [];
  let oldNo = 0;
  let newNo = 0;
  for (const line of raw.split("\n")) {
    if (line.startsWith("@@")) {
      const match = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line);
      if (match) {
        oldNo = parseInt(match[1]!, 10);
        newNo = parseInt(match[2]!, 10);
      }
      continue;
    }
    if (
      line.startsWith("diff ") ||
      line.startsWith("index ") ||
      line.startsWith("--- ") ||
      line.startsWith("+++ ") ||
      line.startsWith("old mode") ||
      line.startsWith("new mode") ||
      line.startsWith("new file") ||
      line.startsWith("deleted file") ||
      line.startsWith("rename ") ||
      line.startsWith("similarity ")
    ) {
      continue;
    }
    const kind = line[0];
    if (kind === "+") {
      lines.push({ kind: "+", text: line.slice(1), oldNo: null, newNo: newNo++ });
    } else if (kind === "-") {
      lines.push({ kind: "-", text: line.slice(1), oldNo: oldNo++, newNo: null });
    } else if (kind === "\\") {
      continue;
    } else {
      lines.push({
        kind: " ",
        text: line.slice(1),
        oldNo: oldNo++,
        newNo: newNo++,
      });
    }
  }
  return lines;
}

const GUTTER_WIDTH = 38;

function GutterNumber(props: { no: number | null | undefined }) {
  return (
    <Text
      class="flex-none px-1 text-right font-mono text-[10px] text-muted-foreground"
      style={{ width: GUTTER_WIDTH }}
    >
      {props.no == null ? "" : String(props.no)}
    </Text>
  );
}

/// The diff view (`editor_ui::DiffView`): one row per line — old/new line
/// numbers, a change bar, the line's text — with a tinted background on
/// changed rows. Callers cap it with `maxHeight`; longer diffs scroll
/// inside. `lineCount`-agnostic: the caller trims the lines for previews.
export function DiffView(props: { lines: DiffLine[]; maxHeight?: number }) {
  return (
    <View
      class="flex flex-col overflow-hidden"
      style={{ maxHeight: props.maxHeight }}
    >
      {props.lines.map((line) => {
        const added = line.kind === "+";
        const removed = line.kind === "-";
        const tint = added
          ? "bg-primary/10"
          : removed
            ? "bg-destructive/10"
            : undefined;
        return (
          <View
            class={"flex flex-none flex-row items-stretch " + (tint ?? "")}
            style={{ height: DIFF_LINE_HEIGHT }}
          >
            <GutterNumber no={line.oldNo} />
            <GutterNumber no={line.newNo} />
            <View
              class={
                "w-[2px] flex-none " +
                (added ? "bg-primary" : removed ? "bg-destructive" : "")
              }
            />
            <View class="min-w-0 flex-1 flex-row items-center px-2">
              <Text
                class="font-mono text-[11px]"
                style={{
                  color: token(
                    added ? "primary" : removed ? "destructive" : "foreground",
                  ),
                }}
              >
                {line.text === "" ? " " : line.text}
              </Text>
            </View>
          </View>
        );
      })}
    </View>
  );
}
