import React, { memo } from "react";
import { View, Text } from "react-native";
import { MessageSquareText } from "lucide-react-native";
import { theme } from "@/styles/theme";

export const ChatEmptyState = memo(function ChatEmptyState() {
  return (
    <View className="flex-1 items-center justify-center px-8">
      <View
        className="w-14 h-14 rounded-2xl items-center justify-center mb-4"
        style={{ backgroundColor: "rgba(255,255,255,0.06)" }}
      >
        <MessageSquareText size={24} color={theme.colors.text.secondary} />
      </View>
      <Text className="text-foreground text-base font-semibold mb-1.5 text-center">
        Start a conversation
      </Text>
      <Text className="text-foreground-secondary text-sm text-center leading-5">
        Ask the agent to write code, review a change, or run commands on your project.
      </Text>
    </View>
  );
});
