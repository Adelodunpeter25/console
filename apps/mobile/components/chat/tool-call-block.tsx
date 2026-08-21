import React, { memo, useMemo, useState } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView } from "react-native";
import { AlertTriangle, CheckCircle2, ChevronRight, ChevronDown } from "lucide-react-native";
import type { ToolCall, ToolResult } from "@console/types";
import { ToolResultContent } from "./tool-result-content";
import { DiffSummaryBadge } from "./diff-view";
import { getToolMeta, formatUnknown, argSummary, computeLineDiff, computeNewFileDiff } from "../../utils";
import { useAppStore, useSessionStore } from "../../stores";

interface ToolCallBlockProps {
  calls: ToolCall[];
  results?: ToolResult[];
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
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const sessionCwd = useSessionStore((state) =>
    selectedSessionId ? state.sessions[selectedSessionId]?.sessionCwd : null,
  );
  const meta = getToolMeta(call.name);
  const summary = argSummary(call, sessionCwd);
  const hasResult = Boolean(result);
  const isError = result?.isError;

  const diffStats = useMemo(() => {
    if (!result || result.isError || !call.arguments || typeof call.arguments !== "object") {
      return null;
    }
    const args = call.arguments as Record<string, unknown>;
    if (
      call.name === "editFile" &&
      typeof args.oldContent === "string" &&
      typeof args.newContent === "string"
    ) {
      return computeLineDiff(args.oldContent, args.newContent);
    }
    if (call.name === "writeFile" && typeof args.content === "string") {
      return computeNewFileDiff(args.content);
    }
    return null;
  }, [call.name, call.arguments, result]);

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
          {diffStats ? (
            <DiffSummaryBadge
              addedCount={diffStats.addedCount}
              removedCount={diffStats.removedCount}
            />
          ) : null}
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
          {call.arguments != null && !diffStats ? (
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
                callArgs={call.arguments}
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
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const sessionCwd = useSessionStore((state) =>
    selectedSessionId ? state.sessions[selectedSessionId]?.sessionCwd : null,
  );
  const meta = getToolMeta(name);
  const summary = calls.length === 1 ? argSummary(calls[0]!, sessionCwd) : `${calls.length} calls`;

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
