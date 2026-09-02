import React, { useEffect, useCallback } from "react";
import { View, Text, ScrollView, Pressable, BackHandler } from "react-native";
import { Bot, ChevronRight } from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { useSessionSubagents } from "@/hooks";
import { app$, setActiveTab, setSelectedSubagentId } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";

export function SubagentsScreen() {
  const selectedSessionId = useValue(app$.selectedSessionId);
  const { subagents } = useSessionSubagents(selectedSessionId);

  const handleBack = useCallback(() => {
    setActiveTab("chat");
  }, []);

  useEffect(() => {
    const onBackPress = () => {
      handleBack();
      return true;
    };
    const sub = BackHandler.addEventListener("hardwareBackPress", onBackPress);
    return () => sub.remove();
  }, [handleBack]);

  const handleOpenDetail = (subagentId: string) => {
    setSelectedSubagentId(subagentId);
    setActiveTab("subagent-details");
  };

  const title = `Subagents (${subagents.length})`;

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader title={title} onBack={handleBack} />

      <ScrollView
        className="flex-1 px-4 pt-3"
        contentContainerStyle={{ paddingBottom: 40 }}
        showsVerticalScrollIndicator={false}
      >
        {subagents.length === 0 ? (
          <View className="py-20 items-center justify-center gap-2.5 px-6">
            <View className="w-12 h-12 rounded-full bg-[#18181c] items-center justify-center border border-[#27272a] mb-1">
              <Bot size={24} color="#71717a" />
            </View>
            <Text className="text-base font-medium text-[#fafafa]">
              No Subagents Spawned
            </Text>
            <Text className="text-xs text-[#71717a] text-center leading-5">
              Subagents created by the assistant during this session will stream their activity and summaries here in real time.
            </Text>
          </View>
        ) : (
          <View className="gap-3">
            {subagents.map((subagent) => {
              const isRunning = subagent.status === "running";
              const isCompleted = subagent.status === "completed";

              const statusColor = isRunning
                ? "#38bdf8"
                : isCompleted
                ? "#22c55e"
                : "#ef4444";

              const statusText = isRunning
                ? subagent.maxTurns && subagent.maxTurns > 0
                  ? `Running (Turn ${Math.max(1, subagent.currentTurn)}/${subagent.maxTurns})`
                  : "Running"
                : isCompleted
                ? "Done"
                : subagent.status === "aborted"
                ? "Aborted"
                : "Failed";

              return (
                <Pressable
                  key={subagent.subagentId}
                  onPress={() => handleOpenDetail(subagent.subagentId)}
                  className="p-3.5 rounded-xl bg-[#141417] border border-[#27272a] active:bg-[#1a1a1e] gap-2.5"
                >
                  <View className="flex-row items-center justify-between">
                    <View className="flex-row items-center gap-2 flex-1 mr-2">
                      <View className="w-6 h-6 rounded-md bg-[#1c1c20] items-center justify-center border border-[#303036]">
                        <Bot size={13} color={statusColor} />
                      </View>
                      <Text
                        className="text-sm font-semibold text-[#fafafa] flex-1 truncate"
                        numberOfLines={1}
                      >
                        {subagent.role}
                      </Text>
                    </View>

                    <View className="flex-row items-center gap-2">
                      <View
                        className={`px-2.5 py-0.5 rounded-full border ${
                          isRunning
                            ? "bg-[#0284c7]/20 border-[#38bdf8]/40"
                            : isCompleted
                            ? "bg-[#15803d]/20 border-[#22c55e]/40"
                            : "bg-[#991b1b]/20 border-[#ef4444]/40"
                        }`}
                      >
                        <Text
                          className="text-[10.5px] font-semibold"
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

                  <View className="flex-row items-center justify-between pt-2 border-t border-white/[0.04]">
                    <Text className="text-[11px] text-[#71717a]">
                      {subagent.activities.length} tool action
                      {subagent.activities.length === 1 ? "" : "s"}
                    </Text>
                    <Text className="text-[11px] text-[#38bdf8] font-medium">
                      View details →
                    </Text>
                  </View>
                </Pressable>
              );
            })}
          </View>
        )}
      </ScrollView>
    </View>
  );
}
