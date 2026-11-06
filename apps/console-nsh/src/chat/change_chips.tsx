// The changed-file chip row under a turn's final assistant message, ported
// from console-ui's `chat/change_chips.rs`: one chip per file (file-type
// icon, name, `+N -N`, hover diff popover — press-opened here, see
// `./diff_popover`), and a fold toggle ("+N more" / "Show less") once the
// turn touched more files than fit.

import { Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Icon, type IconName } from "../common/icons";
import { WithDiffPopover } from "./diff_popover";

/// A turn's chip row shows this many chips while collapsed; beyond
/// `COLLAPSED_CHIPS + 1` files the rest fold into one "+N more" chip (a
/// "+1 more" chip would hide no more than the file it replaces).
export const COLLAPSED_CHIPS = 3;

/// Whole-session change totals for the pinned summary pill: one entry per
/// unique path, with churn summed across every turn that touched it.
export type ChangesSummary = {
  files: number;
  additions: number;
  deletions: number;
};

export function changesSummaryFrom(
  changes: { path: string; additions: number; deletions: number }[],
): ChangesSummary {
  const paths = new Set<string>();
  const summary: ChangesSummary = { files: 0, additions: 0, deletions: 0 };
  for (const change of changes) {
    if (!paths.has(change.path)) {
      paths.add(change.path);
      summary.files += 1;
    }
    summary.additions += change.additions;
    summary.deletions += change.deletions;
  }
  return summary;
}

export function changesSummaryIsEmpty(summary: ChangesSummary): boolean {
  return summary.files === 0;
}

/// One changed file of a turn, copied out of the server row once when the
/// session's changes arrive.
export type TurnChange = {
  path: string;
  status: string;
  additions: number;
  deletions: number;
  diffText: string | null;
};

export function turnChangeFrom(change: {
  path: string;
  status: string;
  additions: number;
  deletions: number;
  diffText: string | null;
}): TurnChange {
  return {
    path: change.path,
    status: change.status,
    additions: change.additions,
    deletions: change.deletions,
    diffText: change.diffText,
  };
}

/// Every changed file of one turn, in the order the agent touched them.
export type TurnChanges = {
  /// The user message that started the turn; the fold state is keyed by it.
  userMessageId: string;
  files: TurnChange[];
};

/// Basename of a path, handling both separator styles (`base_name`).
export function baseName(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] ?? path;
}

// The desktop resolves file-type icons through console-shared's whole
// extension table; these are the extensions a change row plausibly carries,
// mapped onto the generated `fileType/Name` icons, with the plain file mark
// as the fallback.
const FILE_TYPE_BY_EXTENSION: Record<string, string> = {
  rs: "Rust",
  go: "Go",
  py: "Python",
  ts: "Typescript",
  tsx: "Typescript",
  mts: "Typescript",
  cts: "Typescript",
  js: "Javascript",
  jsx: "Javascript",
  mjs: "Javascript",
  cjs: "Javascript",
  c: "C",
  h: "C",
  cc: "Cpp",
  cpp: "Cpp",
  hpp: "Cpp",
  cs: "Csharp",
  swift: "Swift",
  kt: "Kotlin",
  java: "Java",
  rb: "Ruby",
  php: "Php",
  html: "Html",
  css: "Css",
  scss: "Sass",
  sass: "Sass",
  json: "Json",
  yaml: "Yaml",
  yml: "Yaml",
  toml: "Toml",
  xml: "Xml",
  md: "Markdown",
  sh: "Console",
  sql: "Database",
  graphql: "Graphql",
  gql: "Graphql",
  diff: "Diff",
  patch: "Diff",
  png: "Image",
  jpg: "Image",
  jpeg: "Image",
  gif: "Image",
  webp: "Image",
  svg: "Svg",
  pdf: "Pdf",
  zip: "Zip",
  lock: "Lock",
};

/// The file-type icon name for a path, using the basename.
export function fileTypeIconName(path: string): string {
  const name = baseName(path);
  const dot = name.lastIndexOf(".");
  const ext = dot > 0 ? name.slice(dot + 1).toLowerCase() : "";
  return `fileType/${FILE_TYPE_BY_EXTENSION[ext] ?? "File"}`;
}

/// The fold toggle after a long chip row: "+N more" while collapsed, and the
/// same chip reading "Show less" once expanded. No diff popover.
function MoreChangesChip(props: {
  hidden: number;
  expanded: boolean;
  onToggle?: () => void;
}) {
  return (
    <View
      class="flex-row items-center gap-1.5 rounded-[7px] border border-border px-2 py-[3px] hover:bg-accent"
      role="button"
      accessibilityLabel="Toggle change chips"
      onPress={props.onToggle}
    >
      <Icon
        name={props.expanded ? "Minus" : "Plus"}
        size={12}
        color={token("muted-foreground")}
      />
      <Text class="text-[12px] text-muted-foreground">
        {props.expanded ? "Show less" : `${props.hidden} more`}
      </Text>
    </View>
  );
}

/// One changed-file chip: file-type icon, name, `+N -N`. A deleted file's
/// name is struck through. Pressing opens that file's diff for the turn
/// (through the popover when one exists, directly when not).
function FileChangeChip(props: {
  change: TurnChange;
  onOpen?: (path: string, diffText: string | null) => void;
}) {
  const change = props.change;
  const name = baseName(change.path);
  const deleted = change.status === "deleted";
  // The desktop's theme.success has no NSH palette token; green reads as the
  // accent here (see `common/pill_popovers.tsx`), so additions use `primary`.
  return (
    <WithDiffPopover
      change={change}
      onOpen={props.onOpen}
      trigger={
        <View
          class="flex-row items-center gap-1.5 rounded-[7px] border border-border px-2 py-[3px] hover:bg-accent"
          role="button"
          accessibilityLabel={`Show diff for ${name}`}
          onPress={
            props.onOpen
              ? () => props.onOpen!(change.path, change.diffText)
              : undefined
          }
        >
          <Icon
            name={fileTypeIconName(change.path) as IconName}
            size={14}
            mono={false}
          />
          <Text
            class={
              "text-[12px] " +
              // The desktop's text_tertiary/text_secondary both map to the
              // muted token here.
              (deleted
                ? "text-muted-foreground line-through"
                : "text-muted-foreground")
            }
          >
            {name}
          </Text>
          <View class="flex-row items-center gap-1 text-[11px] font-semibold">
            <Text style={{ color: token("primary") }}>
              {`+${change.additions}`}
            </Text>
            <Text style={{ color: token("destructive") }}>
              {`-${change.deletions}`}
            </Text>
          </View>
        </View>
      }
    />
  );
}

/// Build the chip row for one turn, or nothing when it changed no files.
export function TurnChangeChips(props: {
  turn: TurnChanges;
  /// Whether the row is expanded past its collapsed chips. Pass a thunk when
  /// it can change after mount.
  expanded: boolean | (() => boolean);
  onOpen?: (path: string, diffText: string | null) => void;
  onToggle?: () => void;
}): JSX.Element | null {
  const expanded = () =>
    typeof props.expanded === "function" ? props.expanded() : props.expanded;
  const count = () => props.turn.files.length;
  const foldable = () => count() > COLLAPSED_CHIPS + 1;
  const shown = () =>
    foldable() && !expanded() ? COLLAPSED_CHIPS : count();
  const hidden = () => count() - COLLAPSED_CHIPS;
  if (count() === 0) return null;
  return (
    <View class="w-full flex-row flex-wrap gap-1.5 pt-1">
      {props.turn.files.slice(0, shown()).map((change) => (
        <FileChangeChip change={change} onOpen={props.onOpen} />
      ))}
      {foldable() ? (
        <MoreChangesChip
          hidden={hidden()}
          expanded={expanded()}
          onToggle={props.onToggle}
        />
      ) : null}
    </View>
  );
}
