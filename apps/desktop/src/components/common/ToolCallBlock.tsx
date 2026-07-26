import React from "react";
import { ChevronRight, Wrench, AlertCircle, CheckCircle2 } from "lucide-react";
import type { ToolCall, ToolResult } from "@console/types";
import { formatUnknown } from "../../utils/format";

interface ToolCallBlockProps {
  calls: ToolCall[];
  results?: ToolResult[];
}

/**
 * Collapsible block showing tool calls and their results, styled like
 * Conductor's expandable "tool calls" sections.
 */
export function ToolCallBlock({ calls, results }: ToolCallBlockProps) {
  const [expanded, setExpanded] = React.useState(false);
  const hasResults = results && results.length > 0;
  const hasError = results?.some((r) => r.isError);

  return (
    <div className="rounded-lg border border-tool-border bg-tool overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-2.5 hover:bg-white/5 transition-colors"
      >
        <ChevronRight
          size={14}
          className={`text-foreground-muted transition-transform ${expanded ? "rotate-90" : ""}`}
        />
        <Wrench size={14} className="text-blue-400 shrink-0" />
        <span className="text-xs font-medium text-foreground-secondary">
          {calls.length} tool {calls.length === 1 ? "call" : "calls"}
          {hasResults && results ? `, ${results.length} result${results.length === 1 ? "" : "s"}` : ""}
        </span>
        {hasError ? (
          <AlertCircle size={14} className="text-danger ml-auto shrink-0" />
        ) : hasResults ? (
          <CheckCircle2 size={14} className="text-success ml-auto shrink-0" />
        ) : null}
      </button>

      {expanded && (
        <div className="border-t border-tool-border divide-y divide-border">
          {calls.map((call, i) => {
            const result = results?.find((r) => r.toolCallId === call.id);
            return (
              <div key={call.id ?? i} className="px-3 py-2.5">
                <div className="flex items-center gap-2 mb-1.5">
                  <span className="text-xs font-mono font-semibold text-blue-400">
                    {call.name}
                  </span>
                  {result?.isError ? (
                    <AlertCircle size={12} className="text-danger" />
                  ) : result ? (
                    <CheckCircle2 size={12} className="text-success" />
                  ) : null}
                </div>
                {call.arguments != null && (
                  <pre className="text-xs font-mono text-foreground-muted whitespace-pre-wrap break-all mb-1.5 bg-black/30 rounded p-2">
                    {formatUnknown(call.arguments)}
                  </pre>
                )}
                {result && (
                  <pre
                    className={`text-xs font-mono whitespace-pre-wrap break-all bg-black/30 rounded p-2 mt-1.5 ${
                      result.isError ? "text-danger" : "text-foreground-secondary"
                    }`}
                  >
                    {formatUnknown(result.content)}
                  </pre>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
