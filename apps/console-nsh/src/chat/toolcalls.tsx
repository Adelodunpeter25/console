// Grouped tool-call activity for the transcript, ported from console-ui's
// `chat/toolcalls.rs`: one collapsible run summary, consecutive calls grouped
// by tool name, and independently expandable arguments/results. Results are
// always matched by `toolCallId`, never by their position in the transcript.
//
// Deviations from the Rust source, until their dependencies are ported:
// - `ThinkingBlock` lives in another migration pass; thinking rows render a
//   local disclosure with the same collapsed-by-default behaviour.
// - The editor diff views (`EditorDiff`/`file_diff_block`) live behind the
//   concurrent diff-popover port; edit rows render the diff's changed lines
//   as colored monospace text instead.
// - console-core's `argument_summary`/`argument_path` and the markdown
//   selection/link plumbing are not ported; summary/path/label helpers are
//   local subsets, and selection is dropped.

import { Text, View, token, signal } from "nsh";
import type { JSX } from "nsh";
import { Icon, type IconName } from "../common/icons";
import { MarkdownBlocks, METRICS } from "./markdown/render";
import { formatWorkingElapsed } from "../utils/time";
import { WorkingIndicator } from "./working_indicator";
import { ThinkingBlock } from "./thinking_block";

/// Tallest an edit row's diff grows before it scrolls inside the row.
const EDIT_DIFF_MAX_HEIGHT = 240;

// Kind flag bits matching the server's tools.ToolKindFlags
// (apps/server-go/internal/agent/tools/classify.go).
const KIND_EDIT = 1;
const KIND_WRITE = 2;
const KIND_READ = 4;
const KIND_FILE_TARGET = 8;
const KIND_SUBAGENT = 16;

export interface ToolCall {
  id: string;
  name: string;
  arguments: unknown;
  /// Server-derived display label; falls back to the raw name.
  label?: string | null;
  kindFlags?: number | null;
}

export interface ToolResult {
  toolCallId: string;
  content: unknown;
  isError?: boolean | null;
}

/// A tool call and the result, when the backend has returned one.
export interface ToolCallEntry {
  call: ToolCall;
  result?: ToolResult | null;
}

/// An ordered step in a run's activity timeline.
export type ActivityEvent =
  | { type: "text"; id: string; text: string }
  | { type: "thinking"; id: string; text: string }
  | { type: "toolCall"; entry: ToolCallEntry };

/// One file's diff inside a file-targeting call (a `batchWrite` yields one
/// per written file, a `writeFile`/`editFile` a single-element list).
export interface FileDiff {
  path: string;
  added: number;
  removed: number;
  lines: { kind: "+" | "-" | " "; text: string }[];
}

function kindFlag(call: ToolCall, flag: number): boolean {
  return (call.kindFlags ?? 0) !== 0 && (call.kindFlags! & flag) !== 0;
}
export const isEdit = (call: ToolCall) => kindFlag(call, KIND_EDIT);
export const isWrite = (call: ToolCall) => kindFlag(call, KIND_WRITE);
export const isRead = (call: ToolCall) => kindFlag(call, KIND_READ);
export const isFileTarget = (call: ToolCall) => kindFlag(call, KIND_FILE_TARGET);
export const isSubagent = (call: ToolCall) => kindFlag(call, KIND_SUBAGENT);

export function displayLabel(call: ToolCall): string {
  return call.label || call.name;
}

/// console-core's `ToolCallEntry` join: attach separately persisted results
/// to their calls by stable backend ID.
export function attachResultsById(
  entries: ToolCallEntry[],
  results: ToolResult[],
): ToolCallEntry[] {
  const byId = new Map(results.map((result) => [result.toolCallId, result]));
  return entries.map((entry) => ({
    ...entry,
    result: byId.get(entry.call.id) ?? null,
  }));
}

/// The path a file-targeting call acts on, from the common argument keys.
export function argumentPath(arguments_: unknown): string | undefined {
  if (arguments_ == null || typeof arguments_ !== "object") return undefined;
  const object = arguments_ as Record<string, unknown>;
  for (const key of ["path", "file_path", "filePath"]) {
    const value = object[key];
    if (typeof value === "string" && value.length > 0) return value;
  }
  return undefined;
}

/// A one-line summary for the tool row: the file path when there is one, the
/// shell action, or the first string argument (a subset of console-core's
/// `argument_summary`, which is not ported yet).
export function argumentSummary(
  call: ToolCall,
  cwd?: string | null,
): string | undefined {
  const object = call.arguments as Record<string, unknown> | null;
  if (object == null || typeof object !== "object") return undefined;
  const string = (key: string) =>
    typeof object[key] === "string" ? (object[key] as string) : undefined;
  const path = argumentPath(call.arguments);
  if (path) {
    let relative = path;
    if (cwd && path.startsWith(cwd)) relative = path.slice(cwd.length + 1);
    return truncate(relative, 72);
  }
  if (call.name === "bashJob" || call.name === "bash_job") {
    const action = string("action") ?? "status";
    const jobId = string("jobId");
    return truncate(jobId ? `${action} ${jobId}` : action, 72);
  }
  if (call.name === "loadTools") return string("group")?.slice(0, 72);
  for (const key of ["command", "query", "url", "name"]) {
    const value = string(key);
    if (value) return truncate(value, 72);
  }
  return undefined;
}

function truncate(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

/// A minimal line diff (common prefix/suffix around a changed middle): the
/// full editor diff renderer lands with the diff-popover port.
function lineDiff(oldText: string, newText: string): FileDiff["lines"] {
  const oldLines = oldText.split("\n");
  const newLines = newText.split("\n");
  let start = 0;
  while (
    start < oldLines.length &&
    start < newLines.length &&
    oldLines[start] === newLines[start]
  ) {
    start += 1;
  }
  let endOld = oldLines.length;
  let endNew = newLines.length;
  while (
    endOld > start &&
    endNew > start &&
    oldLines[endOld - 1] === newLines[endNew - 1]
  ) {
    endOld -= 1;
    endNew -= 1;
  }
  const lines: FileDiff["lines"] = [];
  for (const text of oldLines.slice(start, endOld)) {
    lines.push({ kind: "-", text });
  }
  for (const text of newLines.slice(start, endNew)) {
    lines.push({ kind: "+", text });
  }
  return lines;
}

/// console-core's `file_call_diffs`: per-file diffs for edit/write calls.
export function fileCallDiffs(
  name: string,
  arguments_: unknown,
): FileDiff[] {
  const args = arguments_ as Record<string, unknown> | null;
  if (args == null || typeof args !== "object") return [];
  const str = (key: string) =>
    typeof args[key] === "string" ? (args[key] as string) : undefined;
  if (name === "batchWrite") {
    const files = Array.isArray(args.files) ? args.files : [];
    return files.map((file) => {
      const object = file as Record<string, unknown>;
      const path = String(object.path ?? "");
      const content = typeof object.content === "string" ? object.content : "";
      const lines = lineDiff("", content);
      return {
        path,
        added: lines.filter((l) => l.kind === "+").length,
        removed: lines.filter((l) => l.kind === "-").length,
        lines,
      };
    });
  }
  const path = argumentPath(arguments_) ?? "";
  if (name === "editFile") {
    const lines = lineDiff(str("oldText") ?? str("old") ?? "", str("newText") ?? str("new") ?? "");
    return [{
      path,
      added: lines.filter((l) => l.kind === "+").length,
      removed: lines.filter((l) => l.kind === "-").length,
      lines,
    }];
  }
  if (name === "writeFile") {
    const lines = lineDiff("", str("content") ?? "");
    return [{
      path,
      added: lines.filter((l) => l.kind === "+").length,
      removed: lines.filter((l) => l.kind === "-").length,
      lines,
    }];
  }
  return [];
}

/// Actions the component reports so an owning transcript can record state
/// alongside its own (the desktop's `ToolCallsAction`).
export type ToolCallsAction =
  | { type: "toggleRun"; runId: string; expanded: boolean }
  | { type: "toggleCall"; callId: string; expanded: boolean }
  | { type: "toggleThinking"; id: string; expanded: boolean }
  | { type: "viewSubagentInPanel"; callId: string };

/// The generic result text: unwrap nested `content`, prefer `.text`, join
/// string arrays, pretty-print anything else (the desktop's
/// `formatted_result`).
export function formattedResult(result: ToolResult): string {
  let content = result.content;
  while (
    content != null &&
    typeof content === "object" &&
    !Array.isArray(content) &&
    "content" in content
  ) {
    content = (content as Record<string, unknown>).content;
  }
  if (typeof content === "string") return content;
  if (content != null && typeof content === "object" && !Array.isArray(content)) {
    const text = (content as Record<string, unknown>).text;
    if (typeof text === "string") return text;
  }
  if (Array.isArray(content)) {
    return content
      .map((item) =>
        typeof item === "string"
          ? item
          : item != null &&
              typeof item === "object" &&
              typeof (item as Record<string, unknown>).text === "string"
            ? ((item as Record<string, unknown>).text as string)
            : "",
      )
      .filter((text) => text.length > 0)
      .join("\n");
  }
  try {
    return JSON.stringify(content, null, 2) ?? String(content);
  } catch {
    return String(content);
  }
}

export function normalizeUnixTimestamp(timestamp: number): number {
  return timestamp > 10_000_000_000 ? Math.floor(timestamp / 1000) : timestamp;
}

/// console-core's `elapsed_seconds`: the run's duration in whole seconds.
export function elapsedSeconds(
  startedAt?: number | null,
  endedAt?: number | null,
): number {
  if (startedAt == null || endedAt == null) return 0;
  const start = normalizeUnixTimestamp(startedAt);
  const end = normalizeUnixTimestamp(endedAt);
  return Math.max(0, end - start);
}

interface ToolGroup {
  name: string;
  entries: ToolCallEntry[];
}

/// Consecutive calls of the same tool collapse into one group (the desktop's
/// `groups` pass).
function groupEntries(entries: ToolCallEntry[]): ToolGroup[] {
  const groups: ToolGroup[] = [];
  for (const entry of entries) {
    const last = groups[groups.length - 1];
    if (last && last.name === entry.call.name) {
      last.entries.push(entry);
      continue;
    }
    groups.push({ name: entry.call.name, entries: [entry] });
  }
  return groups;
}

/// A grouped, collapsible run activity surface. The disclosure state is
/// reactive and owned here (the desktop stored it in the transcript entity's
/// `ToolCallsState`); `onAction` still reports every change so an owner can
/// mirror it.
export function ToolCalls(props: {
  runId: string;
  events: ActivityEvent[];
  working: boolean | (() => boolean);
  startedAt?: number | null;
  elapsedSeconds?: number;
  /// Session working directory, so absolute tool paths render relative.
  cwd?: string | null;
  onAction?: (action: ToolCallsAction) => void;
}) {
  const working = () =>
    typeof props.working === "function" ? props.working() : props.working;
  // Disclosure sets, tracked so the rows re-render on toggle.
  const expandedRuns = signal(new Set<string>());
  const collapsedRuns = signal(new Set<string>());
  const expandedCalls = signal(new Set<string>());
  const thinkingExpanded = signal(new Set<string>());

  const toggleCall = (callId: string) => {
    const open = expandedCalls().has(callId);
    const next = new Set(expandedCalls());
    if (open) next.delete(callId);
    else next.add(callId);
    expandedCalls.set(next);
    props.onAction?.({ type: "toggleCall", callId, expanded: !open });
  };

  const runId = props.runId;

  const statusFor = (
    entry: ToolCallEntry,
  ): { icon: IconName; color: string; pending: boolean } => {
    if (entry.result?.isError) {
      return { icon: "Alert", color: token("destructive"), pending: false };
    }
    if (entry.result) {
      return { icon: "CircleCheck", color: token("primary"), pending: false };
    }
    return { icon: "LoaderCircle", color: token("muted-foreground"), pending: true };
  };

  const callRow = (entry: ToolCallEntry, first: boolean): JSX.Element => {
    const callId = entry.call.id;
    const open = expandedCalls().has(callId);
    const summary = argumentSummary(entry.call, props.cwd);
    const status = statusFor(entry);
    const filePath = argumentPath(entry.call.arguments);
    const diffs =
      isEdit(entry.call) || isWrite(entry.call)
        ? fileCallDiffs(entry.call.name, entry.call.arguments)
        : [];
    const hasDiff = diffs.length > 0;
    const totalAdded = diffs.reduce((sum, d) => sum + d.added, 0);
    const totalRemoved = diffs.reduce((sum, d) => sum + d.removed, 0);
    return (
      <View
        class={
          "w-full flex-col " + (first ? "" : "border-t border-border")
        }
      >
        <View
          class="flex-row items-center gap-2 rounded-md border border-transparent bg-muted px-[10px] py-[6px]"
          role="button"
          accessibilityLabel={`Tool call ${displayLabel(entry.call)}`}
          onPress={() => toggleCall(callId)}
        >
          {isFileTarget(entry.call) && filePath ? (
            <Icon name="File" size={13} color={token("muted-foreground")} />
          ) : (
            <Icon
              name={activityIconForCall(entry.call)}
              size={13}
              color={token("muted-foreground")}
            />
          )}
          <Text class="flex-none text-[12px] font-medium text-muted-foreground">
            {displayLabel(entry.call)}
          </Text>
          {summary ? (
            <View class="min-w-0 flex-1 overflow-hidden">
              <Text class="font-mono text-[11.5px] text-muted-foreground">
                {summary}
              </Text>
            </View>
          ) : null}
          {hasDiff ? (
            <View class="flex-row items-center gap-1">
              <Text class="text-[10px] font-semibold text-primary">
                +{totalAdded}
              </Text>
              <Text class="text-[10px] font-semibold text-destructive">
                -{totalRemoved}
              </Text>
            </View>
          ) : null}
          {isSubagent(entry.call) ? (
            <View
              class="flex-row items-center rounded bg-accent px-1.5 py-0.5"
              role="button"
              accessibilityLabel={`View subagent ${callId} in panel`}
              onPress={() =>
                props.onAction?.({ type: "viewSubagentInPanel", callId })
              }
            >
              <Text class="text-[10.5px] font-medium text-foreground">
                View in Panel →
              </Text>
            </View>
          ) : null}
          <View class="ml-auto flex-none flex-row items-center gap-1.5">
            <Icon name={status.icon} size={13} color={status.color} />
            {/* Reserve the chevron's width so this row's status icon aligns
                with group rows, which show a chevron after it. */}
            <View class="w-3" />
          </View>
        </View>
        {open ? (
          <View class="flex-col gap-1.5 px-[10px] pb-2">
            {hasDiff
              ? diffs.map((fileDiff) => (
                  <View
                    class="flex-col gap-0.5 rounded bg-muted px-2 py-1.5"
                    style={{ maxHeight: EDIT_DIFF_MAX_HEIGHT }}
                  >
                    <View class="flex-row items-center gap-2">
                      <Text class="min-w-0 flex-1 overflow-hidden font-mono text-[11px] text-foreground">
                        {fileDiff.path}
                      </Text>
                      <Text class="text-[10px] font-semibold text-primary">
                        +{fileDiff.added}
                      </Text>
                      <Text class="text-[10px] font-semibold text-destructive">
                        -{fileDiff.removed}
                      </Text>
                    </View>
                    {fileDiff.lines.map((line, index) => (
                      <Text
                        class="font-mono text-[11px] text-muted-foreground"
                        style={
                          line.kind === "+"
                            ? { color: token("primary") }
                            : line.kind === "-"
                              ? { color: token("destructive") }
                              : undefined
                        }
                      >
                        {`${line.kind} ${line.text}`}
                      </Text>
                    ))}
                  </View>
                ))
              : (
                  <Section
                    label="Arguments"
                    text={prettyJson(entry.call.arguments)}
                  />
                )}
            {entry.result ? (
              <Section label="Result" text={formattedResult(entry.result)} />
            ) : null}
          </View>
        ) : null}
      </View>
    );
  };

  const groupRow = (group: ToolGroup): JSX.Element => {
    const groupKey = `group:${group.entries[0].call.id}`;
    const open = expandedCalls().has(groupKey);
    const failed = group.entries.some((entry) => entry.result?.isError);
    const complete = group.entries.every((entry) => entry.result != null);
    const status = failed
      ? { icon: "Alert" as IconName, color: token("destructive") }
      : complete
        ? { icon: "CircleCheck" as IconName, color: token("primary") }
        : { icon: "LoaderCircle" as IconName, color: token("muted-foreground") };
    return (
      <View class="w-full flex-col">
        <View
          class="flex-row items-center gap-2 rounded-md border border-transparent bg-muted px-[10px] py-[6px]"
          role="button"
          accessibilityLabel={`Tool group ${displayLabel(group.entries[0].call)}`}
          onPress={() => toggleCall(groupKey)}
        >
          <Icon
            name={activityIconForCall(group.entries[0].call)}
            size={13}
            color={token("muted-foreground")}
          />
          <Text class="flex-none text-[12px] font-medium text-muted-foreground">
            {displayLabel(group.entries[0].call)}
          </Text>
          <Text class="text-[11.5px] text-muted-foreground">
            {`· ${group.entries.length} calls`}
          </Text>
          <View class="ml-auto flex-none flex-row items-center gap-1.5">
            <Icon
              name={status.icon}
              size={13}
              color={status.color}
            />
            <Icon
              name={open ? "ChevronUp" : "ChevronRight"}
              size={12}
              color={token("muted-foreground")}
            />
          </View>
        </View>
        {open ? (
          <View class="border-t border-border pl-[10px]">
            {group.entries.map((entry, index) =>
              callRow(entry, index === 0),
            )}
          </View>
        ) : null}
      </View>
    );
  };

  // Thinking is collapsed by default, like the desktop app. `ThinkingBlock`
  // owns the header and word count; the toggle state is ours (the desktop
  // kept it in the transcript's `thinking_expanded`).
  const thinkingRow = (id: string, text: string): JSX.Element => {
    const open = () => thinkingExpanded().has(id);
    return (
      <View class="py-1.5">
        <ThinkingBlock
          id={id}
          text={text}
          collapsed={() => !open()}
          onToggle={(expanded) => {
            const next = new Set(thinkingExpanded());
            if (expanded) next.add(id);
            else next.delete(id);
            thinkingExpanded.set(next);
            props.onAction?.({ type: "toggleThinking", id, expanded });
          }}
        />
      </View>
    );
  };

  const textRow = (_id: string, text: string): JSX.Element => (
    <View class="py-1.5">
      <MarkdownBlocks source={text} metrics={METRICS.BODY} />
    </View>
  );

  // The expanded timeline: thinking/text events render individually, and
  // consecutive tool calls of the same name collapse into one group
  // (the desktop's `groupEvents` pass).
  const timelineChildren = (): JSX.Element[] => {
    const children: JSX.Element[] = [];
    let pending: ToolCallEntry[] = [];
    const flush = (): JSX.Element[] =>
      pending.length === 0
        ? []
        : groupEntries(pending).flatMap((group) =>
            group.entries.length === 1
              ? [callRow(group.entries[0], true)]
              : [groupRow(group)],
          );
    for (const event of props.events) {
      if (event.type === "toolCall") {
        pending = [...pending, event.entry];
        continue;
      }
      children.push(...flush());
      pending = [];
      children.push(
        event.type === "thinking"
          ? thinkingRow(event.id, event.text)
          : textRow(event.id, event.text),
      );
    }
    children.push(...flush());
    return children;
  };

  // A run the agent is still working through is expanded unless the user
  // collapsed it; a finished run is collapsed unless the user expanded it.
  const expanded = () =>
    working()
      ? !collapsedRuns().has(runId)
      : expandedRuns().has(runId);
  const toggleRun = () => {
    const open = expanded();
    if (working()) {
      const next = new Set(collapsedRuns());
      if (open) next.add(runId);
      else next.delete(runId);
      collapsedRuns.set(next);
    } else {
      const next = new Set(expandedRuns());
      if (open) next.delete(runId);
      else next.add(runId);
      expandedRuns.set(next);
    }
    props.onAction?.({ type: "toggleRun", runId, expanded: !open });
  };

  if (props.events.length === 0) return null;

  const elapsed = formatWorkingElapsed(props.elapsedSeconds ?? 0);

  return (
    <View class="w-full border-b border-border">
      <View
        class="w-full flex-row items-center gap-[7px] rounded-md px-[10px] py-[5px]"
        role="button"
        accessibilityLabel={`Run ${runId}`}
        onPress={toggleRun}
      >
        {working() ? (
          <WorkingIndicator startedAt={props.startedAt ?? Math.floor(Date.now() / 1000)} />
        ) : (
          <>
            {/* Tool call errors are already shown per-row with the alert icon;
                the run summary reflects the run's overall duration, not
                failure — the agent may have recovered and completed. */}
            <Text class="text-[11.5px] text-muted-foreground">
              {`Worked for ${elapsed}`}
            </Text>
            <Icon
              name={expanded() ? "ChevronUp" : "ChevronRight"}
              size={12}
              color={token("muted-foreground")}
            />
          </>
        )}
      </View>
      {expanded() ? (
        <View class="overflow-hidden">{timelineChildren()}</View>
      ) : null}
    </View>
  );
}

function Section(props: { label: string; text: string }) {
  return (
    <View class="flex-col gap-[3px]">
      <Text class="text-[10px] font-semibold text-muted-foreground">
        {props.label}
      </Text>
      <View class="overflow-hidden rounded bg-muted px-2 py-1.5">
        <Text class="font-mono text-[12px] leading-[17px] text-muted-foreground">
          {props.text}
        </Text>
      </View>
    </View>
  );
}

/// The monochrome glyph per tool (a subset of console-ui's
/// `activity_icon_for_call`).
function activityIconForCall(call: ToolCall): IconName {
  if (isEdit(call) || isWrite(call)) return "FileDiff";
  if (isRead(call)) return "File";
  if (isSubagent(call)) return "Sparkle";
  if (call.name.includes("bash") || call.name.includes("shell")) {
    return "Terminal";
  }
  return "Wrench";
}

function prettyJson(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? "";
  } catch {
    return String(value);
  }
}
