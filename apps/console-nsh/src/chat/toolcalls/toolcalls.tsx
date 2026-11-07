// Grouped tool-call activity for the transcript, ported from console-ui's
// `chat/toolcalls.rs`: one collapsible run summary, consecutive calls grouped
// by tool name, and independently expandable arguments/results. Results are
// always matched by `toolCallId`, never by their position in the transcript.
//
// Deviations from the Rust source, until their dependencies are ported:
// - Edit rows render their diffs through the shared `DiffView`
//   (`../diff_view`); syntax highlighting waits for the markdown code-block
//   hooks.
// - console-core's `argument_summary`/`argument_path` and the markdown
//   selection/link plumbing are not ported; summary/path/label helpers live
//   in `model.ts` as local subsets, and selection is dropped.

import { Text, View, signal, token } from "nsh";
import { Icon } from "../../common/icons";
import type { JSX } from "nsh";
import { formatWorkingElapsed } from "../../utils/time";
import { WorkingIndicator } from "../working_indicator";
import { ThinkingBlock } from "../thinking_block";
import { MarkdownBlocks, METRICS } from "../markdown/render";
import { callRow, groupRow } from "./rows";
import type {
  ActivityEvent,
  ToolCallEntry,
  ToolCallsAction,
} from "./model";

// The model is the type surface callers use (`ToolCall`, `ToolResult`,
// `ActivityEvent`, …); re-exported so imports keep pointing at one module.
export * from "./model";

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

  const ctx = {
    cwd: props.cwd,
    expanded: (key: string) => expandedCalls().has(key),
    toggleCall,
    onAction: props.onAction,
  };

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
              ? [callRow(group.entries[0], true, ctx)]
              : [groupRow(group, ctx)],
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
