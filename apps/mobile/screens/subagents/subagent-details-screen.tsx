import React, { useState, useEffect, useCallback, useMemo } from "react";
import { View, Text, ScrollView, Pressable, BackHandler } from "react-native";
import {
  Bot,
  CheckCircle2,
  AlertTriangle,
  Copy,
  Check,
} from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { MarkdownRenderer } from "@/components/common/markdown-renderer";
import { useSessionSubagents } from "@/hooks";
import { app$, setActiveTab } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";
import { setStringAsync } from "expo-clipboard";

export function SubagentDetailsScreen() {
  const selectedSessionId = useValue(app$.selectedSessionId);
  const selectedSubagentId = useValue(app$.selectedSubagentId);
  const { subagents } = useSessionSubagents(selectedSessionId);
  const [copied, setCopied] = useState(false);

  const subagent = useMemo(
    () => subagents.find((s) => s.subagentId === selectedSubagentId),
    [subagents, selectedSubagentId],
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
                    : "bg-[#991b1b]/20 border-[#ef4444]/40"
                }`}
              >
                <Text
                  className="text-xs font-semibold"
                  style={{ color: statusColor }}
                >
                  {isRunning
                    ? "Running"
                    : isCompleted
                    ? "Completed"
                    : subagent.status === "aborted"
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
                  {subagent.activities.map((act) => {
                    const actRunning = act.status === "running";
                    const actDone = act.status === "completed";

                    const argsSummary = (() => {
                      const a = act.args as Record<string, unknown> | undefined;
                      if (!a) return null;
                      // Ordered by priority — first match wins
                      const val =
                        a.command ??
                        a.CommandLine ??
                        a.path ??
                        a.AbsolutePath ??
                        a.SearchDirectory ??
                        a.TargetFile ??
                        a.pattern ??
                        a.Pattern ??
                        a.Query ??
                        a.query ??
                        a.url ??
                        a.Url ??
                        a.question ??
                        a.directory ??
                        a.SearchPath ??
                        a.Prompt ??
                        a.prompt ??
                        a.filePath ??
                        a.targetFile ??
                        a.absolutePath ??
                        a.content;
                      if (val != null) {
                        const s = String(val);
                        return s.length > 60 ? s.slice(0, 57) + "…" : s;
                      }
                      // Fallback: show first key=value pair
                      const firstKey = Object.keys(a)[0];
                      if (firstKey) {
                        const fv = String(a[firstKey]).slice(0, 40);
                        return `${firstKey}: ${fv}`;
                      }
                      return null;
                    })();

                    return (
                      <View
                        key={act.toolCallId}
                        className="flex-row items-center gap-2.5 p-2.5 rounded-xl bg-[#141417] border border-[#27272a]"
                      >
                        {actRunning ? (
                          <Bot size={13} color="#38bdf8" />
                        ) : actDone ? (
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
                  <MarkdownRenderer content={subagent.summary} />
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
