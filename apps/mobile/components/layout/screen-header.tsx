import React from "react";
import { Text, View, Pressable } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { ChevronLeft, Settings } from "lucide-react-native";

interface ScreenHeaderProps {
  title: string;
  onBack?: () => void;
  showSettings?: boolean;
  onSettingsPress?: () => void;
}

export function ScreenHeader({ title, onBack, showSettings, onSettingsPress }: ScreenHeaderProps) {
  const insets = useSafeAreaInsets();
  // Tight gap under the status bar: system inset + a small breathing room.
  const paddingTop = insets.top + 6;

  return (
    <View
      className="flex-row justify-between items-center px-4 pb-2.5"
      style={{ paddingTop }}
    >
      <View className="flex-row items-center flex-1 min-w-0 mr-2">
        {onBack ? (
          <Pressable
            className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center mr-3"
            style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
            onPress={onBack}
          >
            <ChevronLeft size={20} color="#ffffff" />
          </Pressable>
        ) : null}
        <Text
          className="text-[22px] font-bold text-foreground tracking-tight flex-shrink"
          numberOfLines={1}
        >
          {title}
        </Text>
      </View>

      {showSettings ? (
        <Pressable
          className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
          style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
          onPress={onSettingsPress}
        >
          <Settings size={18} color="#ffffff" />
        </Pressable>
      ) : null}
    </View>
  );
}
