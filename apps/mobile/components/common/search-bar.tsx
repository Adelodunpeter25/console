import React from "react";
import { View, TextInput, TouchableOpacity } from "react-native";
import { Search, Plus } from "lucide-react-native";

interface SearchBarProps {
  value: string;
  onChangeText: (text: string) => void;
  onComposePress?: () => void;
  placeholder?: string;
}

export function SearchBar({
  value,
  onChangeText,
  onComposePress,
  placeholder = "Search threads",
}: SearchBarProps) {
  return (
    <View className="flex-row items-center px-4 pt-3 pb-6 bg-screen/95 border-t border-border">
      <View className="flex-1 flex-row items-center bg-card border border-border rounded-full px-4 h-12 mr-3">
        <Search size={18} color="#71717a" />
        <TextInput
          className="flex-1 ml-2.5 text-foreground text-sm h-full"
          placeholder={placeholder}
          placeholderTextColor="#71717a"
          value={value}
          onChangeText={onChangeText}
          autoCapitalize="none"
          autoCorrect={false}
        />
      </View>
      <TouchableOpacity
        className="w-12 h-12 rounded-full bg-foreground items-center justify-center"
        onPress={onComposePress}
        activeOpacity={0.8}
      >
        <Plus size={20} color="#000000" />
      </TouchableOpacity>
    </View>
  );
}
