import React from "react";
import { View, TextInput, Pressable, StyleSheet } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { KeyboardStickyView, useKeyboardState } from "react-native-keyboard-controller";
import { Search } from "lucide-react-native";
import { HugeiconsIcon } from "@hugeicons/react-native";
import { PencilEdit02Icon } from "@hugeicons/core-free-icons";

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
  const keyboardVisible = useKeyboardState((s) => s.isVisible);
  // Home-indicator clearance when idle; sit flush above the keys when open.
  // KeyboardStickyView translates the whole bar with the keyboard (needed on
  // edge-to-edge Android where adjustResize does not shrink the window).
  const paddingBottom = keyboardVisible ? 10 : Math.max(insets.bottom, 10) + 6;

  return (
    <KeyboardStickyView offset={{ closed: 0, opened: 0 }}>
      <View
        className="flex-row items-center px-4 pt-3 bg-screen border-t border-border"
        style={{ paddingBottom }}
      >
        <View className="flex-1 flex-row items-center bg-card border border-border rounded-full px-4 h-12 mr-3">
          <Search size={18} color="#71717a" />
          <TextInput
            style={styles.input}
            placeholder={placeholder}
            placeholderTextColor="#71717a"
            value={value}
            onChangeText={onChangeText}
            autoCapitalize="none"
            autoCorrect={false}
            returnKeyType="search"
            blurOnSubmit={false}
            textAlignVertical="center"
          />
        </View>
        <Pressable
          className={`w-12 h-12 rounded-full bg-foreground items-center justify-center ${disabled ? "opacity-50" : ""}`}
          style={({ pressed }) => (pressed && !disabled ? { opacity: 0.8 } : null)}
          onPress={onComposePress}
          disabled={disabled}
        >
          <HugeiconsIcon icon={PencilEdit02Icon} size={19} color="#000000" />
        </Pressable>
      </View>
    </KeyboardStickyView>
  );
}

const styles = StyleSheet.create({
  input: {
    flex: 1,
    marginLeft: 10,
    height: "100%",
    color: "#fafafa",
    fontSize: 14,
    // Android otherwise adds extra font padding that shifts placeholder up.
    includeFontPadding: false,
    paddingVertical: 0,
  },
});
