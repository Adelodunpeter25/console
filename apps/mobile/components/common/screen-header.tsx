import React from "react";
import { Text, View, TouchableOpacity } from "react-native";
import { ChevronLeft, Settings } from "lucide-react-native";

interface ScreenHeaderProps {
  title: string;
  onBack?: () => void;
  showSettings?: boolean;
  onSettingsPress?: () => void;
}

export function ScreenHeader({
  title,
  onBack,
  showSettings,
  onSettingsPress,
}: ScreenHeaderProps) {
  return (
    <View className="flex-row justify-between items-center px-4 pt-4 pb-3">
      <View className="flex-row items-center flex-1">
        {onBack ? (
          <TouchableOpacity
            className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center mr-3"
            onPress={onBack}
            activeOpacity={0.7}
          >
            <ChevronLeft size={20} color="#ffffff" />
          </TouchableOpacity>
        ) : null}
        <Text className="text-[22px] font-bold text-foreground tracking-tight">{title}</Text>
      </View>

      {showSettings ? (
        <TouchableOpacity
          className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
          onPress={onSettingsPress}
          activeOpacity={0.7}
        >
          <Settings size={18} color="#ffffff" />
        </TouchableOpacity>
      ) : null}
    </View>
  );
}
