import React, { forwardRef, useState, useMemo } from "react";
import { View, Text, ScrollView, Pressable } from "react-native";
import type { BottomSheetModal } from "@gorhom/bottom-sheet";
import {
  Bot,
  CheckCircle2,
  AlertTriangle,
  ChevronRight,
  ChevronLeft,
  Copy,
  Check,
} from "lucide-react-native";
import type { SubagentInfo } from "@console/types";
import { setStringAsync } from "expo-clipboard";
import { SharedBottomSheet } from "@/components/common/shared-bottom-sheet";
import { MarkdownRenderer } from "@/components/common/markdown-renderer";

interface SubagentBottomSheetProps {
  subagents: SubagentInfo[];
  selectedSubagentId?: string | null;
  onSelectSubagent?: (id: string | null) => void;
}

export const SubagentBottomSheet = forwardRef<BottomSheetModal, SubagentBottomSheetProps>(
  function SubagentBottomSheet(
    { subagents, selectedSubagentId, onSelectSubagent },
    ref,
  ) {
    const [localSelectedId, setLocalSelectedId] = useState<string | null>(null);
    const [copied, setCopied] = useState(false);

    const activeId = selectedSubagentId ?? localSelectedId;
    const activeSubagent = useMemo(
      () => subagents.find((s) => s.subagentId === activeId),
      [subagents, activeId],
    );

    const handleSelect = (id: string | null) => {
      setLocalSelectedId(id);
      onSelectSubagent?.(id);
    };

    const handleCopySummary = async (text: string) => {
      await setStringAsync(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    };

    const title = activeSubagent
      ? activeSubagent.role
      : `Subagents (${subagents.length})`;

    return (
      <SharedBottomSheet
        ref={ref}
        title={title}
        snapPoints={["60%", "90%"]}
      >
        <ScrollView
          className="flex-1"
          contentContainerStyle={{ paddingBottom: 36 }}
          showsVerticalScrollIndicator={false}
        >
          {activeSubagent ? (
            // --- Detailed Subagent View ---
            <View className="gap-3.5 pt-1">
              {/* Back to list button */}
              <Pressable
                onPress={() => handleSelect(null)}
                className="flex-row items-center gap-1.5 py-1 px-2 -ml-2 self-start rounded-md active:bg-white/[0.06]"
              >
                <ChevronLeft size={16} color="#a1a1aa" />
                <Text className="text-xs font-medium text-[#a1a1aa]">
                  All Subagents
                </Text>
              </Pressable>

              {/* Status Header Box */}
              <View className="p-3 rounded-xl bg-[#141417] border border-[#27272a] flex-row items-center justify-between">
                <View className="flex-row items-center gap-2.5">
                  <View className="w-7 h-7 rounded-lg bg-[#1c1c20] items-center justify-center border border-[#303036]">
                    <Bot
                      size={15}
                      color={
                        activeSubagent.status === "running"
                          ? "#38bdf8"
                          : activeSubagent.status === "completed"
                          ? "#22c55e"
                          : "#f87171"
                      }
                    />
                  </View>
                  <View>
                    <Text className="text-xs font-semibold text-[#fafafa]">
                      {activeSubagent.role}
                    </Text>
                    <Text className="text-[11px] text-[#71717a]">
                      {activeSubagent.name}
                    </Text>
                  </View>
                </View>

                {/* Status Pill */}
                <View
                  className={`px-2.5 py-1 rounded-full border ${
                    activeSubagent.status === "running"
                      ? "bg-[#0284c7]/20 border-[#38bdf8]/40"
                      : activeSubagent.status === "completed"
                      ? "bg-[#15803d]/20 border-[#22c55e]/40"
                      : "bg-[#991b1b]/20 border-[#ef4444]/40"
                  }`}
                >
                  <Text
                    className={`text-[11px] font-semibold ${
                      activeSubagent.status === "running"
                        ? "text-[#38bdf8]"
                        : activeSubagent.status === "completed"
                        ? "text-[#22c55e]"
                        : "text-[#ef4444]"
                    }`}
                  >
                    {activeSubagent.status === "running"
                      ? activeSubagent.maxTurns && activeSubagent.maxTurns > 0
                        ? `Running (${Math.max(1, activeSubagent.currentTurn)}/${activeSubagent.maxTurns})`
                        : "Running"
                      : activeSubagent.status === "completed"
                      ? "Completed"
                      : activeSubagent.status === "aborted"
                      ? "Aborted"
                      : "Failed"}
                  </Text>
                </View>
              </View>

              {/* 1. Mission Prompt */}
              <View className="gap-1.5">
                <Text className="text-[10.5px] font-bold uppercase tracking-wider text-[#71717a]">
                  Mission Prompt
                </Text>
                <View className="p-3 rounded-xl bg-[#141417] border border-[#27272a]">
                  <Text className="text-xs text-[#d4d4d8] leading-5" selectable>
                    {activeSubagent.prompt}
                  </Text>
                </View>
              </View>

              {/* 2. Activity Timeline */}
              <View className="gap-1.5">
                <Text className="text-[10.5px] font-bold uppercase tracking-wider text-[#71717a]">
                  Activity Timeline ({activeSubagent.activities.length})
                </Text>
                {activeSubagent.activities.length === 0 ? (
                  <View className="p-3 rounded-xl bg-[#141417] border border-[#27272a]">
                    <Text className="text-xs text-[#71717a]">
                      No tool actions executed yet...
                    </Text>
                  </View>
                ) : (
                  <View className="gap-2">
                    {activeSubagent.activities.map((act) => {
                      const isRunning = act.status === "running";
                      const isDone = act.status === "completed";

                      const argsSummary = act.args
                        ? (act.args as Record<string, unknown>).command ||
                          (act.args as Record<string, unknown>).CommandLine ||
                          (act.args as Record<string, unknown>).path ||
                          (act.args as Record<string, unknown>).AbsolutePath ||
                          (act.args as Record<string, unknown>).TargetFile ||
                          (act.args as Record<string, unknown>).pattern ||
                          (act.args as Record<string, unknown>).Query ||
                          (act.args as Record<string, unknown>).query
                        : null;

                      return (
                        <View
                          key={act.toolCallId}
                          className="flex-row items-center gap-2.5 p-2.5 rounded-xl bg-[#141417] border border-[#27272a]"
                        >
                          {isRunning ? (
                            <Bot size={13} color="#38bdf8" />
                          ) : isDone ? (
                            <CheckCircle2 size={13} color="#22c55e" />
                          ) : (
                            <AlertTriangle size={13} color="#ef4444" />
                          )}

                          <View className="px-1.5 py-0.5 rounded bg-[#222226] border border-[#33333a]">
                            <Text className="text-[10.5px] font-mono font-medium text-[#fafafa]">
                              {act.toolName}
                            </Text>
                          </View>

                          {argsSummary ? (
                            <Text
                              className="text-xs font-mono text-[#a1a1aa] flex-1 truncate"
                              numberOfLines={1}
                              ellipsizeMode="tail"
                            >
                              {String(argsSummary)}
                            </Text>
                          ) : null}

                          {act.error ? (
                            <Text className="text-[10px] text-[#ef4444] shrink-0">
                              Error
                            </Text>
                          ) : null}
                        </View>
                      );
                    })}
                  </View>
                )}
              </View>

              {/* 3. Summary Section */}
              <View className="gap-1.5">
                <View className="flex-row items-center justify-between">
                  <Text className="text-[10.5px] font-bold uppercase tracking-wider text-[#71717a]">
                    Summary
                  </Text>
                  {activeSubagent.summary ? (
                    <Pressable
                      onPress={() => handleCopySummary(activeSubagent.summary!)}
                      className="flex-row items-center gap-1.5 px-2 py-1 rounded-md bg-[#1c1c20] border border-[#303036] active:bg-[#25252a]"
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

                <View className="p-3.5 rounded-xl bg-[#141417] border border-[#27272a] min-h-[60px]">
                  {activeSubagent.summary ? (
                    <MarkdownRenderer content={activeSubagent.summary} />
                  ) : activeSubagent.status === "running" ? (
                    <Text className="text-xs text-[#71717a] italic">
                      (Awaiting subagent completion...)
                    </Text>
                  ) : activeSubagent.error ? (
                    <Text className="text-xs text-[#ef4444]">
                      {activeSubagent.error}
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
            // --- Subagents List Overview ---
            <View className="gap-2.5 pt-1">
              {subagents.length === 0 ? (
                <View className="py-12 items-center justify-center gap-2">
                  <Bot size={28} color="#52525b" />
                  <Text className="text-sm font-medium text-[#a1a1aa]">
                    No Subagents Spawned
                  </Text>
                  <Text className="text-xs text-[#71717a] text-center px-6">
                    Subagents created during this session will stream activity and summary here in real time.
                  </Text>
                </View>
              ) : (
                subagents.map((subagent) => {
                  const isRunning = subagent.status === "running";
                  const isCompleted = subagent.status === "completed";

                  const statusColor = isRunning
                    ? "#38bdf8"
                    : isCompleted
                    ? "#22c55e"
                    : "#ef4444";

                  const statusText = isRunning
                    ? subagent.maxTurns && subagent.maxTurns > 0
                      ? `Running (${Math.max(1, subagent.currentTurn)}/${subagent.maxTurns})`
                      : "Running"
                    : isCompleted
                    ? "Done"
                    : subagent.status === "aborted"
                    ? "Aborted"
                    : "Failed";

                  return (
                    <Pressable
                      key={subagent.subagentId}
                      onPress={() => handleSelect(subagent.subagentId)}
                      className="p-3.5 rounded-xl bg-[#141417] border border-[#27272a] active:bg-[#1a1a1e] gap-2"
                    >
                      <View className="flex-row items-center justify-between">
                        <View className="flex-row items-center gap-2 flex-1 mr-2">
                          <View className="w-6 h-6 rounded-md bg-[#1c1c20] items-center justify-center border border-[#303036]">
                            <Bot size={13} color={statusColor} />
                          </View>
                          <Text
                            className="text-xs font-semibold text-[#fafafa] flex-1 truncate"
                            numberOfLines={1}
                          >
                            {subagent.role}
                          </Text>
                        </View>

                        <View className="flex-row items-center gap-2">
                          <View
                            className={`px-2 py-0.5 rounded-full border ${
                              isRunning
                                ? "bg-[#0284c7]/20 border-[#38bdf8]/40"
                                : isCompleted
                                ? "bg-[#15803d]/20 border-[#22c55e]/40"
                                : "bg-[#991b1b]/20 border-[#ef4444]/40"
                            }`}
                          >
                            <Text
                              className="text-[10px] font-semibold"
                              style={{ color: statusColor }}
                            >
                              {statusText}
                            </Text>
                          </View>
                          <ChevronRight size={14} color="#71717a" />
                        </View>
                      </View>

                      <Text
                        className="text-xs text-[#a1a1aa] leading-4"
                        numberOfLines={2}
                      >
                        {subagent.prompt}
                      </Text>

                      <View className="flex-row items-center justify-between pt-1 border-t border-white/[0.04]">
                        <Text className="text-[10.5px] text-[#71717a]">
                          {subagent.activities.length} tool action
                          {subagent.activities.length === 1 ? "" : "s"}
                        </Text>
                        <Text className="text-[10.5px] text-[#38bdf8] font-medium">
                          View details →
                        </Text>
                      </View>
                    </Pressable>
                  );
                })
              )}
            </View>
          )}
        </ScrollView>
      </SharedBottomSheet>
    );
  },
);
