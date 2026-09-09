import React, { useState, useEffect, useCallback, useMemo } from "react";
import { View, Text, ScrollView, Pressable, BackHandler } from "react-native";
import {
  Bot,
  CheckCircle2,
  AlertTriangle,
  Copy,
  Check,
  ChevronDown,
} from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { MarkdownRenderer } from "@/components/common/markdown-renderer";
import { useSessionSubagents } from "@/hooks";
import { app$, setActiveTab } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";
import { setStringAsync } from "expo-clipboard";
import type { SubagentInfo } from "@console/types";

type SubagentActivity = SubagentInfo["activities"][number];

type ActivityGroup = {
  toolName: string;
  activities: SubagentActivity[];
};

function groupActivities(activities: SubagentActivity[]): ActivityGroup[] {
  const groups: ActivityGroup[] = [];
  for (const activity of activities) {
    const last = groups[groups.length - 1];
    if (last?.toolName === activity.toolName) {
      last.activities.push(activity);
    } else {
      groups.push({ toolName: activity.toolName, activities: [activity] });
    }
  }
  return groups;
}

export function SubagentDetailsScreen() {
  const selectedSessionId = useValue(app$.selectedSessionId);
  const selectedSubagentId = useValue(app$.selectedSubagentId);
  const { subagents } = useSessionSubagents(selectedSessionId);
  const [isReady, setIsReady] = useState(false);

  useEffect(() => {
    // Defer heavy markdown rendering until screen mount & transition completes
    const timer = requestAnimationFrame(() => setIsReady(true));
    return () => cancelAnimationFrame(timer);
  }, []);

  const [copied, setCopied] = useState(false);

  const subagent = useMemo(
    () => subagents.find((s) => s.subagentId === selectedSubagentId),
    [subagents, selectedSubagentId],
  );

  const activityGroups = useMemo(
    () => (subagent ? groupActivities(subagent.activities) : []),
    [subagent],
  );

  const handleBack = useCallback(() => {
    setActiveTab("subagents");
  }, []);

  useEffect(() => {
    const onBackPress = () => {
      handleBack();
      return true;
    };
    const sub = BackHandler.addEventListener("hardwareBackPress", onBackPress);
    return () => sub.remove();
  }, [handleBack]);

  const handleCopySummary = async (text: string) => {
    await setStringAsync(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const isRunning = subagent?.status === "running";
  const isCompleted = subagent?.status === "completed";

  const statusColor = isRunning
    ? "#38bdf8"
    : isCompleted
      ? "#22c55e"
      : "#ef4444";

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader
        title={subagent?.role ?? "Subagent Details"}
        onBack={handleBack}
      />

      <ScrollView
        className="flex-1 px-4 pt-3"
        contentContainerStyle={{ paddingBottom: 40 }}
        showsVerticalScrollIndicator={false}
      >
        {subagent ? (
          <View className="gap-4">
            {/* Status Header Box */}
            <View className="p-3.5 rounded-xl bg-[#141417] border border-[#27272a] flex-row items-center justify-between">
              <View className="flex-row items-center gap-2.5">
                <View className="w-8 h-8 rounded-lg bg-[#1c1c20] items-center justify-center border border-[#303036]">
                  <Bot size={16} color={statusColor} />
                </View>
                <View>
                  <Text className="text-sm font-semibold text-[#fafafa]">
                    {subagent.role}
                  </Text>
                  <Text className="text-[11px] text-[#71717a]">
                    {subagent.name}
                  </Text>
                </View>
              </View>

              {/* Status Pill */}
              <View
                className={`px-2.5 py-1 rounded-full border ${
                  isRunning
                    ? "bg-[#0284c7]/20 border-[#38bdf8]/40"
                    : isCompleted
                      ? "bg-[#15803d]/20 border-[#22c55e]/40"
                      : "bg-[#b91c1c]/20 border-[#ef4444]/40"
                }`}
              >
                <Text
                  className={`text-[10px] font-bold uppercase ${
                    isRunning
                      ? "text-[#38bdf8]"
                      : isCompleted
                        ? "text-[#22c55e]"
                        : "text-[#ef4444]"
                  }`}
                >
                  {subagent.status}
                </Text>
              </View>
            </View>

            {/* 1. Mission Prompt */}
            <View className="gap-1.5">
              <Text className="text-[10.5px] font-bold uppercase tracking-wider text-[#71717a]">
                Mission Prompt
              </Text>
              <View className="p-3.5 rounded-xl bg-[#141417] border border-[#27272a]">
                <Text className="text-xs text-[#d4d4d8] leading-5" selectable>
                  {subagent.prompt}
                </Text>
              </View>
            </View>

            {/* 2. Activity Timeline */}
            <View className="gap-1.5">
              <Text className="text-[10.5px] font-bold uppercase tracking-wider text-[#71717a]">
                Activity Timeline ({subagent.activities.length})
              </Text>
              {subagent.activities.length === 0 ? (
                <View className="p-3.5 rounded-xl bg-[#141417] border border-[#27272a]">
                  <Text className="text-xs text-[#71717a]">
                    No tool actions executed yet...
                  </Text>
                </View>
              ) : (
                <View className="gap-2">
                  {activityGroups.map((group, groupIndex) => (
                    <ActivityGroupCard
                      key={`${group.toolName}-${groupIndex}`}
                      group={group}
                      defaultOpen={isRunning && groupIndex === activityGroups.length - 1}
                    />
                  ))}
                </View>
              )}
            </View>

            {/* 3. Summary Section */}
            <View className="gap-1.5">
              <View className="flex-row items-center justify-between">
                <Text className="text-[10.5px] font-bold uppercase tracking-wider text-[#71717a]">
                  Summary
                </Text>
                {subagent.summary ? (
                  <Pressable
                    onPress={() => handleCopySummary(subagent.summary!)}
                    className="flex-row items-center gap-1.5 px-2.5 py-1 rounded-md bg-[#1c1c20] border border-[#303036] active:bg-[#25252a]"
                  >
                    {copied ? (
                      <Check size={11} color="#22c55e" />
                    ) : (
                      <Copy size={11} color="#a1a1aa" />
                    )}
                    <Text className="text-[10px] font-semibold text-[#fafafa]">
                      {copied ? "Copied" : "Copy"}
                    </Text>
                  </Pressable>
                ) : null}
              </View>

              <View className="p-3.5 rounded-xl bg-[#141417] border border-[#27272a] min-h-[70px]">
                {subagent.summary ? (
                  isReady ? (
                    <MarkdownRenderer content={subagent.summary} />
                  ) : (
                    <Text className="text-xs text-[#71717a]">Loading summary...</Text>
                  )
                ) : isRunning ? (
                  <Text className="text-xs text-[#71717a] italic">
                    (Awaiting subagent completion...)
                  </Text>
                ) : subagent.error ? (
                  <Text className="text-xs text-[#ef4444]">
                    {subagent.error}
                  </Text>
                ) : (
                  <Text className="text-xs text-[#71717a]">
                    No summary available.
                  </Text>
                )}
              </View>
            </View>
          </View>
        ) : (
          <View className="py-20 items-center justify-center gap-2">
            <Text className="text-sm text-[#71717a]">Subagent not found.</Text>
          </View>
        )}
      </ScrollView>
    </View>
  );
}

const ActivityGroupCard = React.memo(function ActivityGroupCard({
  group,
  defaultOpen = false,
}: {
  group: ActivityGroup;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const hasError = group.activities.some((activity) => activity.status !== "completed" && activity.status !== "running");
  const isRunning = group.activities.some((activity) => activity.status === "running");
  const completedCount = group.activities.filter((activity) => activity.status === "completed").length;

  return (
    <View className="rounded-xl bg-[#141417] border border-[#27272a] overflow-hidden">
      <Pressable
        onPress={() => setOpen((value) => !value)}
        className="flex-row items-center gap-2.5 p-2.5"
        style={({ pressed }) => ({ opacity: pressed ? 0.75 : 1 })}
      >
        {isRunning ? (
          <Bot size={13} color="#38bdf8" />
        ) : hasError ? (
          <AlertTriangle size={13} color="#ef4444" />
        ) : (
          <CheckCircle2 size={13} color="#22c55e" />
        )}
        <View className="px-1.5 py-0.5 rounded bg-[#222226] border border-[#33333a]">
          <Text className="text-[10.5px] font-mono font-medium text-[#fafafa]">
            {group.toolName}
          </Text>
        </View>
        <Text className="text-xs text-[#a1a1aa] flex-1">
          {group.activities.length} {group.activities.length === 1 ? "call" : "calls"}
        </Text>
        <Text className="text-[10px] text-[#71717a]">
          {isRunning ? "Running" : `${completedCount}/${group.activities.length}`}
        </Text>
        <ChevronDown
          size={13}
          color="#71717a"
          style={{ transform: [{ rotate: open ? "0deg" : "-90deg" }] }}
        />
      </Pressable>

      {open ? (
        <View className="border-t border-[#27272a] px-2 pb-2">
          {group.activities.map((activity) => (
            <ActivityRow key={activity.toolCallId} activity={activity} />
          ))}
        </View>
      ) : null}
    </View>
  );
});

const ActivityRow = React.memo(function ActivityRow({
  activity,
}: {
  activity: SubagentActivity;
}) {
  const argsSummary = activity.summary ?? (() => {
    const a = activity.args as Record<string, unknown> | undefined;
    if (!a) return null;
    const val =
      a.command ?? a.CommandLine ?? a.path ?? a.AbsolutePath ?? a.SearchDirectory ??
      a.TargetFile ?? a.pattern ?? a.Pattern ?? a.Query ?? a.query ?? a.url ?? a.Url ??
      a.question ?? a.directory ?? a.SearchPath ?? a.Prompt ?? a.prompt ?? a.filePath ??
      a.targetFile ?? a.absolutePath;
    if (val != null) {
      const s = String(val);
      return s.length > 60 ? s.slice(0, 57) + "…" : s;
    }
    const firstKey = Object.keys(a)[0];
    if (firstKey) return `${firstKey}: ${String(a[firstKey]).slice(0, 40)}`;
    return null;
  })();
  const isRunning = activity.status === "running";
  const isDone = activity.status === "completed";

  return (
    <View className="flex-row items-center gap-2.5 border-b border-[#27272a] last:border-b-0 py-2">
      {isRunning ? (
        <Bot size={13} color="#38bdf8" />
      ) : isDone ? (
        <CheckCircle2 size={13} color="#22c55e" />
      ) : (
        <AlertTriangle size={13} color="#ef4444" />
      )}
      <View className="px-1.5 py-0.5 rounded bg-[#222226] border border-[#33333a]">
        <Text className="text-[10.5px] font-mono font-medium text-[#fafafa]">
          {activity.toolName}
        </Text>
      </View>
      {argsSummary ? (
        <Text className="text-xs font-mono text-[#a1a1aa] flex-1" numberOfLines={1} ellipsizeMode="tail">
          {String(argsSummary)}
        </Text>
      ) : null}
      {activity.error ? <Text className="text-[10px] text-[#ef4444]">Error</Text> : null}
    </View>
  );
});
