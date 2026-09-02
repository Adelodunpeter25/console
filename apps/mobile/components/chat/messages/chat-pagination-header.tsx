import React, { memo } from "react";
import { View, Text, Pressable, ActivityIndicator } from "react-native";
import { theme } from "@/styles/theme";

interface ChatPaginationHeaderProps {
  hasEarlierMessages: boolean;
  isFetchingEarlierMessages: boolean;
  onFetchEarlierMessages: () => void;
}

export const ChatPaginationHeader = memo(function ChatPaginationHeader({
  hasEarlierMessages,
  isFetchingEarlierMessages,
  onFetchEarlierMessages,
}: ChatPaginationHeaderProps) {
  if (!hasEarlierMessages) return null;

  return (
    <View className="py-2.5 items-center justify-center">
      {isFetchingEarlierMessages ? (
        <View className="flex-row items-center gap-2 py-1">
          <ActivityIndicator size="small" color={theme.colors.text.muted} />
          <Text className="text-xs text-foreground-secondary">Loading earlier messages…</Text>
        </View>
      ) : (
        <Pressable
          onPress={onFetchEarlierMessages}
          className="py-1.5 px-3.5 rounded-full bg-surfaceElevated border border-border/60"
          style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        >
          <Text className="text-xs font-medium text-foreground-secondary">Load earlier messages</Text>
        </Pressable>
      )}
    </View>
  );
});
