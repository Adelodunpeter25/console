import React from "react";
import {
  Wrench,
  AlertCircle,
  CheckCircle2,
  Loader2,
  FileText,
  FilePlus,
  Files,
  SquarePen,
  Terminal,
  Search,
  FolderTree,
  Globe,
  HelpCircle,
  ListTodo,
  Sparkles,
} from "lucide-react";
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

const TOOL_META: Record<string, { label: string; icon: React.ElementType }> = {
  readFile: { label: "Read File", icon: FileText },
  writeFile: { label: "Write File", icon: FilePlus },
  batchWrite: { label: "Batch Write", icon: Files },
  editFile: { label: "Edit File", icon: SquarePen },
  bash: { label: "Run Command", icon: Terminal },
  grep: { label: "Search Code", icon: Search },
  glob: { label: "Find Files", icon: FolderTree },
  listDir: { label: "List Directory", icon: FolderTree },
  fetch: { label: "Fetch URL", icon: Globe },
  webSearch: { label: "Web Search", icon: Globe },
  subagent: { label: "Subagent", icon: Sparkles },
  ask: { label: "Ask Question", icon: HelpCircle },
  todo: { label: "Todo", icon: ListTodo },
};

function getToolMeta(name: string) {
  return TOOL_META[name] ?? { label: name, icon: Wrench };
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
  const Icon = meta.icon;
  const summary = argSummary(call);
  const hasResult = !!result;
  const isError = result?.isError;

  return (
    <div className="border-b border-border last:border-b-0">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 bg-white/[0.02] px-3 py-2 text-left transition-colors hover:bg-white/[0.06]"
      >
        <Icon size={14} className="text-blue-400 shrink-0" />
        <span className="text-xs font-medium text-foreground-secondary shrink-0">
          {meta.label}
        </span>
        {summary && (
          <span className="text-xs font-mono text-foreground-muted truncate">
            {summary}
          </span>
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
        <div className="px-3 pb-2.5 space-y-1.5">
          {call.arguments != null && (
            <div>
              <p className="text-[10px] uppercase tracking-wide text-foreground-muted mb-1">
                Arguments
              </p>
              <pre className="text-xs font-mono text-foreground-secondary whitespace-pre-wrap break-all bg-black/30 rounded p-2 max-h-48 overflow-y-auto selectable-text">
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
                    ? (call.arguments as Record<string, unknown>).path as string
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
  // Tool calls are collapsed by default — the user clicks to expand.
  return (
    <div className="overflow-hidden rounded-lg border border-white/[0.08] bg-white/[0.015]">
      {calls.map((call, i) => {
        const result = results?.find((r) => r.toolCallId === call.id);
        return (
          <ToolCallRow
            key={call.id ?? i}
            call={call}
            result={result}
            defaultOpen={false}
          />
        );
      })}
    </div>
  );
}
