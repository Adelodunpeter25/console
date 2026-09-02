import React, { forwardRef } from "react";
import { View, Text, ScrollView } from "react-native";
import type { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Check, Clock, Circle } from "lucide-react-native";
import type { TodoItem } from "@console/types";
import { SharedBottomSheet } from "@/components/common/shared-bottom-sheet";

interface TodoBottomSheetProps {
  todoItems: TodoItem[];
  completedCount: number;
  totalCount: number;
}

export const TodoBottomSheet = forwardRef<BottomSheetModal, TodoBottomSheetProps>(
  function TodoBottomSheet({ todoItems, completedCount, totalCount }, ref) {
    return (
      <SharedBottomSheet
        ref={ref}
        title={`Tasks (${completedCount}/${totalCount})`}
        snapPoints={["50%", "85%"]}
      >
        <ScrollView
          className="flex-1"
          contentContainerStyle={{ paddingBottom: 32 }}
          showsVerticalScrollIndicator={false}
        >
          <View className="gap-2.5 pt-1">
            {todoItems.map((item, index) => {
              const isDone =
                item.status === "completed" ||
                (item.status as string) === "done" ||
                (item.status as string) === "complete";
              const isInProgress = item.status === "in_progress";

              return (
                <View
                  key={`${item.id ?? index}-${item.content}`}
                  className={`flex-row items-center gap-3 p-3 rounded-xl border ${
                    isInProgress
                      ? "bg-[#18181c] border-[#3f3f46]"
                      : isDone
                      ? "bg-[#121214]/60 border-[#222226]"
                      : "bg-[#141417] border-[#27272a]"
                  }`}
                >
                  {/* Custom Checkbox Status Icon */}
                  <View
                    className={`w-5 h-5 rounded-md items-center justify-center border ${
                      isDone
                        ? "bg-[#14532d]/40 border-[#22c55e]"
                        : isInProgress
                        ? "bg-[#1e293b] border-[#38bdf8]"
                        : "bg-[#18181b] border-[#3f3f46]"
                    }`}
                  >
                    {isDone ? (
                      <Check size={12} color="#22c55e" strokeWidth={3} />
                    ) : isInProgress ? (
                      <View className="w-2 h-2 rounded-full bg-[#38bdf8]" />
                    ) : null}
                  </View>

                  {/* Task Content */}
                  <View className="flex-1">
                    <Text
                      className={`text-sm leading-5 ${
                        isDone
                          ? "line-through text-[#71717a]"
                          : isInProgress
                          ? "text-[#fafafa] font-medium"
                          : "text-[#d4d4d8]"
                      }`}
                    >
                      {item.content}
                    </Text>
                  </View>

                  {/* Status Pill */}
                  {isInProgress ? (
                    <View className="px-2 py-0.5 rounded-full bg-[#0284c7]/20 border border-[#38bdf8]/30">
                      <Text className="text-[10px] font-semibold text-[#38bdf8]">
                        In Progress
                      </Text>
                    </View>
                  ) : isDone ? (
                    <View className="px-2 py-0.5 rounded-full bg-[#15803d]/20 border border-[#22c55e]/30">
                      <Text className="text-[10px] font-semibold text-[#22c55e]">
                        Done
                      </Text>
                    </View>
                  ) : null}
                </View>
              );
            })}
          </View>
        </ScrollView>
      </SharedBottomSheet>
    );
  },
);
