import React from "react";
import { Text, View, TouchableOpacity } from "react-native";
import { ChevronLeft, Settings } from "lucide-react-native";

interface ScreenHeaderProps {
  title: string;
  subtitle?: string;
  onBack?: () => void;
  showSettings?: boolean;
  onSettingsPress?: () => void;
}

export function ScreenHeader({
  title,
  subtitle,
  onBack,
  showSettings,
  onSettingsPress,
}: ScreenHeaderProps) {
  return (
    <View className="flex-row justify-between items-start px-4 pt-4 pb-2">
      <View className="flex-row items-start flex-1">
        {onBack ? (
          <TouchableOpacity className="p-1 -ml-1 mr-2" onPress={onBack}>
            <ChevronLeft size={24} color="#ffffff" />
          </TouchableOpacity>
        ) : null}
        <View>
          <Text className="text-xl font-bold text-white tracking-tight">{title}</Text>
          {subtitle ? (
            <Text className="text-[10px] text-zinc-500 uppercase tracking-[0.2em] mt-0.5">
              {subtitle}
            </Text>
          ) : null}
        </View>
      </View>

      {showSettings ? (
        <TouchableOpacity
          className="p-2 rounded-full active:bg-white/10"
          onPress={onSettingsPress}
        >
          <Settings size={22} color="#ffffff" />
        </TouchableOpacity>
      ) : null}
    </View>
  );
}
