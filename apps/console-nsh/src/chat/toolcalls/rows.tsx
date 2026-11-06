// The tool-call and tool-group rows of the run-activity surface, split out
// of `toolcalls.tsx`. Each row receives a `RowContext` wired to the owning
// `ToolCalls` component's disclosure state.

import { Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Icon, type IconName } from "../../common/icons";
import {
  EDIT_DIFF_MAX_HEIGHT,
  activityIconForCall,
  argumentPath,
  argumentSummary,
  displayLabel,
  fileCallDiffs,
  formattedResult,
  isEdit,
  isFileTarget,
  isSubagent,
  isWrite,
  prettyJson,
  type ToolCallEntry,
  type ToolCallsAction,
} from "./model";

/// What the rows need from the owning `ToolCalls` component: the session
/// cwd, the shared expanded-calls disclosure, and the action reporter.
export interface RowContext {
  cwd?: string | null;
  expanded: (key: string) => boolean;
  toggleCall: (callId: string) => void;
  onAction?: (action: ToolCallsAction) => void;
}

function statusFor(
  entry: ToolCallEntry,
): { icon: IconName; color: string; pending: boolean } {
  if (entry.result?.isError) {
    return { icon: "Alert", color: token("destructive"), pending: false };
  }
  if (entry.result) {
    return { icon: "CircleCheck", color: token("primary"), pending: false };
  }
  return { icon: "LoaderCircle", color: token("muted-foreground"), pending: true };
}

export function callRow(
  entry: ToolCallEntry,
  first: boolean,
  ctx: RowContext,
): JSX.Element {
  const callId = entry.call.id;
  const open = ctx.expanded(callId);
  const summary = argumentSummary(entry.call, ctx.cwd);
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
        onPress={() => ctx.toggleCall(callId)}
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
              ctx.onAction?.({ type: "viewSubagentInPanel", callId })
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
}

export function groupRow(
  group: { name: string; entries: ToolCallEntry[] },
  ctx: RowContext,
): JSX.Element {
  const groupKey = `group:${group.entries[0].call.id}`;
  const open = ctx.expanded(groupKey);
  const failed = group.entries.some((entry) => entry.result?.isError);
  const complete = group.entries.every((entry) => entry.result != null);
  const status = failed
    ? { icon: "Alert" as const, color: token("destructive") }
    : complete
      ? { icon: "CircleCheck" as const, color: token("primary") }
      : { icon: "LoaderCircle" as const, color: token("muted-foreground") };
  return (
    <View class="w-full flex-col">
      <View
        class="flex-row items-center gap-2 rounded-md border border-transparent bg-muted px-[10px] py-[6px]"
        role="button"
        accessibilityLabel={`Tool group ${displayLabel(group.entries[0].call)}`}
        onPress={() => ctx.toggleCall(groupKey)}
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
            callRow(entry, index === 0, ctx),
          )}
        </View>
      ) : null}
    </View>
  );
}

export function Section(props: { label: string; text: string }) {
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
