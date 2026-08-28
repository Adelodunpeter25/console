import React from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { ListTodo, ChevronUp } from "lucide-react-native";

interface TodoBannerProps {
  completedCount: number;
  totalCount: number;
  nextTask?: string;
  onPress: () => void;
}

export function TodoBanner({
  completedCount,
  totalCount,
  nextTask,
  onPress,
}: TodoBannerProps) {
  return (
    <View className="px-4 pb-2">
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="View task list"
        onPress={onPress}
        className="flex-row items-center justify-between px-3.5 py-2.5 rounded-xl bg-[#121214] border border-[#27272a] shadow-sm"
        style={({ pressed }) => ({
          opacity: pressed ? 0.8 : 1,
        })}
      >
        <View className="flex-row items-center gap-2.5 flex-1 mr-2">
          <View className="w-6 h-6 rounded-md bg-[#1c1c20] items-center justify-center border border-[#303036]">
            <ListTodo size={13} color="#a1a1aa" />
          </View>

          <View className="flex-row items-center gap-2 flex-1 min-w-0">
            <View className="flex-row items-center gap-1.5 shrink-0">
              <Text className="text-xs font-bold text-[#fafafa] tracking-wider">
                TASKS
              </Text>
              <View className="px-1.5 py-0.5 rounded-md bg-[#222226] border border-[#33333a]">
                <Text className="text-[10px] font-semibold text-[#a1a1aa]">
                  {completedCount}/{totalCount}
                </Text>
              </View>
            </View>

            {nextTask ? (
              <Text
                className="text-xs text-[#71717a] flex-1 truncate"
                numberOfLines={1}
                ellipsizeMode="tail"
              >
                • {nextTask}
              </Text>
            ) : null}
          </View>
        </View>

        <View className="w-5 h-5 rounded-full bg-[#1c1c20] items-center justify-center border border-[#303036] shrink-0">
          <ChevronUp size={12} color="#a1a1aa" />
        </View>
      </Pressable>
    </View>
  );
}
