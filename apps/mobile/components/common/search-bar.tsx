import React from "react";
import { View, TextInput, Pressable, StyleSheet, Keyboard } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { KeyboardStickyView, useKeyboardState } from "react-native-keyboard-controller";
import { Search, X } from "lucide-react-native";
import { HugeiconsIcon } from "@hugeicons/react-native";
import { PencilEdit02Icon } from "@hugeicons/core-free-icons";
import { theme } from "@/styles/theme";

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

  const handleClear = () => {
    onChangeText("");
    Keyboard.dismiss();
  };

  return (
    <KeyboardStickyView offset={{ closed: 0, opened: 0 }}>
      <View
        className="flex-row items-center px-4 pt-3 bg-screen"
        style={{ paddingBottom }}
      >
        <View className="flex-1 flex-row items-center bg-card border border-border rounded-full px-4 h-12 mr-3">
          <Search size={18} color={theme.colors.text.muted} />
          <TextInput
            style={styles.input}
            placeholder={placeholder}
            placeholderTextColor={theme.colors.text.muted}
            value={value}
            onChangeText={onChangeText}
            autoCapitalize="none"
            autoCorrect={false}
            returnKeyType="search"
            blurOnSubmit={false}
            textAlignVertical="center"
          />
          {value.trim().length > 0 ? (
            <Pressable
              onPress={handleClear}
              hitSlop={8}
              className="w-6 h-6 rounded-full bg-white/10 items-center justify-center ml-1"
              style={({ pressed }) => ({ opacity: pressed ? 0.6 : 1 })}
            >
              <X size={13} color={theme.colors.text.secondary} />
            </Pressable>
          ) : null}
        </View>
        <Pressable
          className={`w-12 h-12 rounded-full items-center justify-center ${disabled ? "opacity-50" : ""}`}
          style={({ pressed }) => ({
            backgroundColor: pressed && !disabled ? "#1a1a1a" : "#000000",
            borderWidth: 1.5,
            borderColor: "rgba(255,255,255,0.18)",
          })}
          onPress={onComposePress}
          disabled={disabled}
        >
          <HugeiconsIcon icon={PencilEdit02Icon} size={19} color="#ffffff" />
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
