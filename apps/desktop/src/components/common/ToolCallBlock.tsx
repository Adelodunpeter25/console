import React from "react";
import { AlertCircle, CheckCircle2, Loader2 } from "lucide-react";
import type { ToolCall, ToolResult } from "@console/types";
import { formatUnknown } from "../../utils/format";
import { ToolResultContent } from "./ToolResultContent";

interface ToolCallBlockProps {
  calls: ToolCall[];
  results?: ToolResult[];
}

/* ------------------------------------------------------------------ */
/* Tool metadata: human-readable label + icon per tool name            */
/* ------------------------------------------------------------------ */

const TOOL_META: Record<string, { label: string }> = {
  readFile: { label: "Read File" },
  writeFile: { label: "Write File" },
  batchWrite: { label: "Batch Write" },
  editFile: { label: "Edit File" },
  bash: { label: "Run Command" },
  grep: { label: "Search Code" },
  glob: { label: "Find Files" },
  listDir: { label: "List Directory" },
  fetch: { label: "Fetch URL" },
  webSearch: { label: "Web Search" },
  subagent: { label: "Subagent" },
  ask: { label: "Ask Question" },
  todo: { label: "Todo" },
};

function getToolMeta(name: string) {
  return TOOL_META[name] ?? { label: name };
}

/** Extract a short summary string from the tool arguments (e.g. file path). */
function argSummary(call: ToolCall): string | null {
  const args = call.arguments;
  if (!args || typeof args !== "object") return null;
  const obj = args as Record<string, unknown>;
  if (typeof obj.path === "string") return obj.path;
  if (typeof obj.filePath === "string") return obj.filePath;
  if (typeof obj.command === "string") {
    const cmd = obj.command as string;
    return cmd.length > 60 ? cmd.slice(0, 57) + "…" : cmd;
  }
  if (typeof obj.pattern === "string") return obj.pattern;
  if (typeof obj.query === "string") {
    const q = obj.query as string;
    return q.length > 60 ? q.slice(0, 57) + "…" : q;
  }
  if (typeof obj.url === "string") return obj.url;
  if (typeof obj.directory === "string") return obj.directory;
  if (typeof obj.question === "string") {
    const q = obj.question as string;
    return q.length > 60 ? q.slice(0, 57) + "…" : q;
  }
  if (Array.isArray(obj.paths) && obj.paths.length > 0) {
    return `${(obj.paths as unknown[]).length} files`;
  }
  if (Array.isArray(obj.operations) && obj.operations.length > 0) {
    return `${(obj.operations as unknown[]).length} operations`;
  }
  return null;
}

/* ------------------------------------------------------------------ */
/* Single tool call row — independently expandable                     */
/* ------------------------------------------------------------------ */

interface ToolCallRowProps {
  call: ToolCall;
  result?: ToolResult;
  defaultOpen?: boolean;
}

const ToolCallRow = React.memo(function ToolCallRow({
  call,
  result,
  defaultOpen = false,
}: ToolCallRowProps) {
  const [open, setOpen] = React.useState(defaultOpen);
  const meta = getToolMeta(call.name);
  const summary = argSummary(call);
  const hasResult = !!result;
  const isError = result?.isError;

  return (
    <div className="border-b border-border last:border-b-0">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 bg-white/[0.02] px-3 py-1.5 text-left transition-colors hover:bg-white/[0.06]"
      >
        <span className="text-xs font-medium text-foreground-secondary shrink-0">{meta.label}</span>
        {summary && (
          <span className="text-xs font-mono text-foreground-muted truncate">{summary}</span>
        )}
        <span className="ml-auto shrink-0">
          {isError ? (
            <AlertCircle size={13} className="text-danger" />
          ) : hasResult ? (
            <CheckCircle2 size={13} className="text-success" />
          ) : (
            <Loader2 size={13} className="text-foreground-muted animate-spin" />
          )}
        </span>
      </button>

      {open && (
        <div className="px-3 pb-2 space-y-1">
          {call.arguments != null && (
            <div>
              <p className="text-[10px] uppercase tracking-wide text-foreground-muted mb-1">
                Arguments
              </p>
              <pre className="text-xs font-mono text-foreground-secondary whitespace-pre-wrap break-all bg-black/30 rounded p-1.5 max-h-32 overflow-y-auto selectable-text">
                {formatUnknown(call.arguments)}
              </pre>
            </div>
          )}
          {result && (
            <div>
              <p className="text-[10px] uppercase tracking-wide text-foreground-muted mb-1">
                Result
              </p>
              <ToolResultContent
                toolName={call.name}
                result={result}
                callFilePath={
                  call.arguments &&
                  typeof call.arguments === "object" &&
                  "path" in (call.arguments as Record<string, unknown>) &&
                  typeof (call.arguments as Record<string, unknown>).path === "string"
                    ? ((call.arguments as Record<string, unknown>).path as string)
                    : undefined
                }
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
});

/* ------------------------------------------------------------------ */
/* Tool call block — renders each call as a named, expandable row       */
/* ------------------------------------------------------------------ */

export function ToolCallBlock({ calls, results }: ToolCallBlockProps) {
  const groups = React.useMemo(() => {
    const grouped = new Map<string, ToolCall[]>();
    for (const call of calls) {
      const group = grouped.get(call.name) ?? [];
      group.push(call);
      grouped.set(call.name, group);
    }
    return [...grouped.entries()].map(([name, groupCalls]) => ({ name, calls: groupCalls }));
  }, [calls]);

  return (
    <div className="overflow-hidden rounded-md border border-white/[0.08] bg-white/[0.015]">
      {groups.map((group) =>
        group.calls.length === 1 ? (
          <ToolCallRow
            key={group.calls[0]!.id}
            call={group.calls[0]!}
            result={results?.find((result) => result.toolCallId === group.calls[0]!.id)}
          />
        ) : (
          <ToolCallGroup key={group.name} name={group.name} calls={group.calls} results={results} />
        ),
      )}
    </div>
  );
}

function ToolCallGroup({
  name,
  calls,
  results,
}: {
  name: string;
  calls: ToolCall[];
  results?: ToolResult[];
}) {
  const [open, setOpen] = React.useState(false);
  const groupResults = calls
    .map((call) => results?.find((result) => result.toolCallId === call.id))
    .filter((result): result is ToolResult => Boolean(result));
  const hasError = groupResults.some((result) => result.isError);
  const complete = groupResults.length === calls.length;
  const meta = getToolMeta(name);
  const summary = calls.length === 1 ? argSummary(calls[0]!) : `${calls.length} calls`;

  return (
    <div className="border-b border-border last:border-b-0">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="flex w-full items-center gap-2 bg-white/[0.02] px-3 py-1.5 text-left transition-colors hover:bg-white/[0.06]"
      >
        <span className="text-xs font-medium text-foreground-secondary">{meta.label}</span>
        {summary && (
          <span className="truncate text-xs font-mono text-foreground-muted">{summary}</span>
        )}
        <span className="ml-auto shrink-0">
          {hasError ? (
            <AlertCircle size={12} className="text-danger" />
          ) : complete ? (
            <CheckCircle2 size={12} className="text-success" />
          ) : (
            <Loader2 size={12} className="text-foreground-muted animate-spin" />
          )}
        </span>
      </button>
      {open && (
        <div className="border-t border-white/[0.06]">
          {calls.map((call) => (
            <ToolCallRow
              key={call.id}
              call={call}
              result={results?.find((result) => result.toolCallId === call.id)}
              defaultOpen={false}
            />
          ))}
        </div>
      )}
    </div>
  );
}
