import React from "react";
import { ChevronRight, ChevronUp, Loader2 } from "lucide-react";
import type { RunActivityState } from "../../types/chat";
import { ToolCallBlock } from "../common/ToolCallBlock";

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

export function RunActivity({ activity, running }: RunActivityProps) {
  const [expanded, setExpanded] = React.useState(running);
  const [now, setNow] = React.useState(Date.now());

  React.useEffect(() => {
    setExpanded(running);
    if (!running) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [running]);

  if (!activity.startedAt && activity.calls.length === 0) return null;

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
        className="flex w-full items-center gap-2 px-1 py-1 text-left text-xs text-foreground-muted hover:text-foreground-secondary"
      >
        {isWorking ? <Loader2 size={13} className="animate-spin" /> : null}
        <span>{summaryLabel}</span>
        {expanded ? <ChevronUp size={14} /> : <ChevronRight size={14} />}
      </button>
      {expanded && activity.calls.length > 0 && (
        <div className="mt-1">
          <ToolCallBlock calls={activity.calls} results={activity.results} />
        </div>
      )}
    </div>
  );
}
