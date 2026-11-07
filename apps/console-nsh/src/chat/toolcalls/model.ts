// The tool-call data model, split out of `toolcalls.tsx`: the wire types,
// kind-flag predicates, and the pure helpers (summaries, diffs, result
// formatting) the run-activity surface renders. Ported from console-ui's
// `chat/toolcalls.rs`.

import type { IconName } from "../../common/icons";
import type { DiffLine } from "../diff_view";

/// Tallest an edit row's diff grows before it scrolls inside the row.
export const EDIT_DIFF_MAX_HEIGHT = 240;

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
  lines: DiffLine[];
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
  // A fresh file has no old side; an emptied file has no new side. Calling
  // lineDiff with an empty string would otherwise yield a phantom removal
  // (or addition) for the empty text.
  if (oldText === "") {
    return newLines.map((text, index) => ({
      kind: "+" as const,
      text,
      oldNo: null,
      newNo: index + 1,
    }));
  }
  if (newText === "") {
    return oldLines.map((text, index) => ({
      kind: "-" as const,
      text,
      oldNo: index + 1,
      newNo: null,
    }));
  }
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
  // Line numbers are 1-based within each file; the changed middle starts
  // after `start` unchanged lines on both sides.
  let oldLine = start;
  for (const text of oldLines.slice(start, endOld)) {
    lines.push({ kind: "-", text, oldNo: ++oldLine, newNo: null });
  }
  let newLine = start;
  for (const text of newLines.slice(start, endNew)) {
    lines.push({ kind: "+", text, oldNo: null, newNo: ++newLine });
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

/// The monochrome glyph per tool (a subset of console-ui's
/// `activity_icon_for_call`).
export function activityIconForCall(call: ToolCall): IconName {
  if (isEdit(call) || isWrite(call)) return "FileDiff";
  if (isRead(call)) return "File";
  if (isSubagent(call)) return "Sparkle";
  if (call.name.includes("bash") || call.name.includes("shell")) {
    return "Terminal";
  }
  return "Wrench";
}

export function prettyJson(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? "";
  } catch {
    return String(value);
  }
}
