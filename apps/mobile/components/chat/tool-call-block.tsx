import React, { memo, useMemo, useState } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView } from "react-native";
import { AlertTriangle, CheckCircle2, ChevronRight, ChevronDown } from "lucide-react-native";
import type { ToolCall, ToolResult } from "@console/types";
import { ToolResultContent } from "./tool-result-content";

interface ToolCallBlockProps {
  calls: ToolCall[];
  results?: ToolResult[];
}

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

function formatUnknown(val: unknown): string {
  if (val === undefined) return "undefined";
  if (val === null) return "null";
  if (typeof val === "string") return val;
  try {
    return JSON.stringify(val, null, 2);
  } catch {
    return String(val);
  }
}

/** Extract a short summary string from the tool arguments (e.g. file path, command, query). */
function argSummary(call: ToolCall): string | null {
  const args = call.arguments;
  if (!args || typeof args !== "object") return null;
  const obj = args as Record<string, unknown>;
  if (typeof obj.path === "string") return obj.path;
  if (typeof obj.filePath === "string") return obj.filePath;
  if (typeof obj.command === "string") {
    const cmd = obj.command as string;
    return cmd.length > 45 ? cmd.slice(0, 42) + "…" : cmd;
  }
  if (typeof obj.pattern === "string") return obj.pattern;
  if (typeof obj.query === "string") {
    const q = obj.query as string;
    return q.length > 45 ? q.slice(0, 42) + "…" : q;
  }
  if (typeof obj.url === "string") return obj.url;
  if (typeof obj.directory === "string") return obj.directory;
  if (typeof obj.question === "string") {
    const q = obj.question as string;
    return q.length > 45 ? q.slice(0, 42) + "…" : q;
  }
  if (Array.isArray(obj.paths) && obj.paths.length > 0) {
    return `${(obj.paths as unknown[]).length} files`;
  }
  if (Array.isArray(obj.operations) && obj.operations.length > 0) {
    return `${(obj.operations as unknown[]).length} operations`;
  }
  return null;
}

interface ToolCallRowProps {
  call: ToolCall;
  result?: ToolResult;
  defaultOpen?: boolean;
}

const ToolCallRow = memo(function ToolCallRow({
  call,
  result,
  defaultOpen = false,
}: ToolCallRowProps) {
  const [open, setOpen] = useState(defaultOpen);
  const meta = getToolMeta(call.name);
  const summary = argSummary(call);
  const hasResult = Boolean(result);
  const isError = result?.isError;

  return (
    <View className="border-b border-white/[0.06] last:border-b-0">
      <Pressable
        onPress={() => setOpen(!open)}
        className="flex-row items-center gap-2 bg-white/[0.02] px-3 py-2"
        style={({ pressed }) => ({
          backgroundColor: pressed ? "rgba(255, 255, 255, 0.06)" : "rgba(255, 255, 255, 0.02)",
        })}
      >
        <Text className="text-xs font-semibold text-foreground-secondary shrink-0">
          {meta.label}
        </Text>
        {summary ? (
          <Text className="text-xs font-mono text-foreground-secondary/70 flex-1" numberOfLines={1}>
            {summary}
          </Text>
        ) : null}
        <View className="ml-auto shrink-0 flex-row items-center gap-1.5">
          {isError ? (
            <AlertTriangle size={13} color="#f87171" />
          ) : hasResult ? (
            <CheckCircle2 size={13} color="#34d399" />
          ) : (
            <ActivityIndicator size="small" color="#71717a" />
          )}
          {open ? (
            <ChevronDown size={13} color="#71717a" />
          ) : (
            <ChevronRight size={13} color="#71717a" />
          )}
        </View>
      </Pressable>

      {open ? (
        <View className="px-3 pb-3 pt-1 gap-2 border-t border-white/[0.04] bg-black/20">
          {call.arguments != null ? (
            <View>
              <Text className="text-[10px] uppercase tracking-wide text-foreground-secondary/70 mb-1 font-semibold">
                Arguments
              </Text>
              <View className="max-h-36 rounded bg-black/40 p-2 overflow-hidden">
                <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
                  <Text className="text-xs font-mono text-foreground-secondary" selectable>
                    {formatUnknown(call.arguments)}
                  </Text>
                </ScrollView>
              </View>
            </View>
          ) : null}
          {result ? (
            <View>
              <Text className="text-[10px] uppercase tracking-wide text-foreground-secondary/70 mb-1 font-semibold">
                Result
              </Text>
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
            </View>
          ) : null}
        </View>
      ) : null}
    </View>
  );
});

const ToolCallGroup = memo(function ToolCallGroup({
  name,
  calls,
  results,
}: {
  name: string;
  calls: ToolCall[];
  results?: ToolResult[];
}) {
  const [open, setOpen] = useState(false);
  const resultsMap = useMemo(() => {
    const map = new Map<string, ToolResult>();
    if (results) for (const r of results) map.set(r.toolCallId, r);
    return map;
  }, [results]);

  const groupResults = useMemo(
    () =>
      calls
        .map((call) => resultsMap.get(call.id))
        .filter((result): result is ToolResult => Boolean(result)),
    [calls, resultsMap],
  );
  const hasError = groupResults.some((result) => result.isError);
  const complete = groupResults.length === calls.length;
  const meta = getToolMeta(name);
  const summary = calls.length === 1 ? argSummary(calls[0]!) : `${calls.length} calls`;

  return (
    <View className="border-b border-white/[0.06] last:border-b-0">
      <Pressable
        onPress={() => setOpen(!open)}
        className="flex-row items-center gap-2 bg-white/[0.02] px-3 py-2"
        style={({ pressed }) => ({
          backgroundColor: pressed ? "rgba(255, 255, 255, 0.06)" : "rgba(255, 255, 255, 0.02)",
        })}
      >
        <Text className="text-xs font-semibold text-foreground-secondary">{meta.label}</Text>
        {summary ? (
          <Text className="text-xs font-mono text-foreground-secondary/70 flex-1" numberOfLines={1}>
            {summary}
          </Text>
        ) : null}
        <View className="ml-auto shrink-0 flex-row items-center gap-1.5">
          {hasError ? (
            <AlertTriangle size={13} color="#f87171" />
          ) : complete ? (
            <CheckCircle2 size={13} color="#34d399" />
          ) : (
            <ActivityIndicator size="small" color="#71717a" />
          )}
          {open ? (
            <ChevronDown size={13} color="#71717a" />
          ) : (
            <ChevronRight size={13} color="#71717a" />
          )}
        </View>
      </Pressable>
      {open ? (
        <View className="border-t border-white/[0.06] bg-black/10">
          {calls.map((call) => (
            <ToolCallRow
              key={call.id}
              call={call}
              result={resultsMap.get(call.id)}
              defaultOpen={false}
            />
          ))}
        </View>
      ) : null}
    </View>
  );
});

export const ToolCallBlock = memo(function ToolCallBlock({
  calls,
  results,
}: ToolCallBlockProps) {
  const resultsMap = useMemo(() => {
    const map = new Map<string, ToolResult>();
    if (results) for (const r of results) map.set(r.toolCallId, r);
    return map;
  }, [results]);

  const groups = useMemo(() => {
    const grouped = new Map<string, ToolCall[]>();
    for (const call of calls) {
      const group = grouped.get(call.name) ?? [];
      group.push(call);
      grouped.set(call.name, group);
    }
    return [...grouped.entries()].map(([name, groupCalls]) => ({ name, calls: groupCalls }));
  }, [calls]);

  return (
    <View className="overflow-hidden rounded-xl border border-white/[0.08] bg-white/[0.015] mb-2">
      {groups.map((group) =>
        group.calls.length === 1 ? (
          <ToolCallRow
            key={group.calls[0]!.id}
            call={group.calls[0]!}
            result={resultsMap.get(group.calls[0]!.id)}
          />
        ) : (
          <ToolCallGroup
            key={group.name}
            name={group.name}
            calls={group.calls}
            results={results}
          />
        ),
      )}
    </View>
  );
});
