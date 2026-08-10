import React from "react";
import { ChevronRight, ChevronUp, Loader2 } from "lucide-react";
import type { ActivityEvent, RunActivityState } from "../../types/chat";
import type { ToolCall, ToolResult } from "@console/types";
import { ToolCallBlock } from "../common/ToolCallBlock";
import { ThinkingBlock } from "../common/ThinkingBlock";
import { MarkdownRenderer } from "../common/MarkdownRenderer";

interface RunActivityProps {
  activity: RunActivityState;
  /** True only for the latest run while the session is actively running. */
  running: boolean;
}

function formatDuration(milliseconds: number): string {
  const seconds = Math.max(0, Math.round(milliseconds / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`;
}

/** A group of consecutive events for rendering. */
type RenderGroup =
  | { kind: "text"; id: string; text: string }
  | { kind: "thinking"; id: string; text: string }
  | { kind: "tools"; id: string; calls: ToolCall[]; results: ToolResult[] };

/**
 * Group consecutive events: text/thinking events render individually,
 * consecutive tool call events of the same tool name render as a single
 * ToolCallBlock.
 */
function groupEvents(events: ActivityEvent[]): RenderGroup[] {
  const groups: RenderGroup[] = [];
  for (const event of events) {
    if (event.type === "text") {
      groups.push({ kind: "text", id: event.id, text: event.text });
    } else if (event.type === "thinking") {
      groups.push({ kind: "thinking", id: event.id, text: event.text });
    } else {
      // toolCall event
      const last = groups[groups.length - 1];
      if (
        last?.kind === "tools" &&
        last.calls.length > 0 &&
        last.calls[last.calls.length - 1]!.name === event.call.name
      ) {
        last.calls.push(event.call);
        if (event.result) last.results.push(event.result);
      } else {
        groups.push({
          kind: "tools",
          id: event.call.id,
          calls: [event.call],
          results: event.result ? [event.result] : [],
        });
      }
    }
  }
  return groups;
}

export function RunActivity({ activity, running }: RunActivityProps) {
  const [expanded, setExpanded] = React.useState(running);
  const [now, setNow] = React.useState(Date.now());

  React.useEffect(() => {
    setExpanded(running);
    if (!running) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [running]);

  const groups = React.useMemo(() => groupEvents(activity.events), [activity.events]);

  // Don't render the activity block at all if there are no tool calls.
  // A simple question/answer with no tool usage should not show "Worked for 0s".
  const hasToolCalls = activity.events.some((e) => e.type === "toolCall");
  if (!hasToolCalls) return null;

  const isWorking = running || activity.status === "working";
  const elapsed = isWorking && activity.startedAt ? now - activity.startedAt : activity.elapsedMs;

  const summaryLabel = isWorking
    ? "Working..."
    : activity.status === "aborted"
      ? `Aborted after ${formatDuration(elapsed)}`
      : activity.status === "failed"
        ? `Failed after ${formatDuration(elapsed)}`
        : `Worked for ${formatDuration(elapsed)}`;

  return (
    <div className="border-b border-white/[0.06] pb-2">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
        className="flex w-full items-center gap-2 px-3 py-1 text-left text-xs text-foreground-muted hover:text-foreground-secondary"
      >
        {isWorking ? <Loader2 size={13} className="animate-spin" /> : null}
        <span>{summaryLabel}</span>
        {expanded ? <ChevronUp size={14} /> : <ChevronRight size={14} />}
      </button>
      {expanded && groups.length > 0 && (
        <div className="mt-1 space-y-2">
          {groups.map((group) => {
            if (group.kind === "thinking") {
              return <ThinkingBlock key={`${group.kind}-${group.id}`} text={group.text} />;
            }
            if (group.kind === "text") {
              return (
                <div key={`${group.kind}-${group.id}`} className="px-1">
                  <MarkdownRenderer content={group.text} />
                </div>
              );
            }
            return (
              <ToolCallBlock
                key={`${group.kind}-${group.id}`}
                calls={group.calls}
                results={group.results}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}
