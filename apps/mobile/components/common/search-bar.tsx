import React from "react";
import { View, TextInput, Pressable } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { Search, Plus } from "lucide-react-native";

interface SearchBarProps {
  value: string;
  onChangeText: (text: string) => void;
  onComposePress?: () => void;
  disabled?: boolean;
  placeholder?: string;
}

export function SearchBar({
  value,
  onChangeText,
  onComposePress,
  disabled = false,
  placeholder = "Search threads",
}: SearchBarProps) {
  const insets = useSafeAreaInsets();
  // Keep clear of the home indicator / nav bar when the keyboard is closed.
  // When the keyboard is open, Android adjustResize shrinks the window and
  // the bottom inset typically drops to 0 so we sit flush above the keys.
  const paddingBottom = Math.max(insets.bottom, 10) + 6;

  return (
    <View
      className="flex-row items-center px-4 pt-3 bg-screen/95 border-t border-border"
      style={{ paddingBottom }}
    >
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
          returnKeyType="search"
          blurOnSubmit={false}
        />
      </View>
      <Pressable
        className={`w-12 h-12 rounded-full bg-foreground items-center justify-center ${disabled ? "opacity-50" : ""}`}
        style={({ pressed }) => (pressed && !disabled ? { opacity: 0.8 } : null)}
        onPress={onComposePress}
        disabled={disabled}
      >
        <Plus size={20} color="#000000" />
      </Pressable>
    </View>
  );
}
