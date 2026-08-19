import React from "react";
import { View, TextInput, Pressable, StyleSheet } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { KeyboardStickyView, useKeyboardState } from "react-native-keyboard-controller";
import { ArrowUp, Square } from "lucide-react-native";
import { theme } from "../../styles/theme";

interface ComposerProps {
  value: string;
  onChangeText: (text: string) => void;
  onSend: () => void;
  onStop?: () => void;
  running?: boolean;
}

/** Chat input row. Shows a stop button while a run is in progress. */
export function Composer({ value, onChangeText, onSend, onStop, running }: ComposerProps) {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((s) => s.isVisible);
  const canSend = value.trim().length > 0;
  // Home-indicator clearance when idle; flush above the keys when open.
  const paddingBottom = keyboardVisible ? 8 : Math.max(insets.bottom, 8) + 4;

  return (
    <KeyboardStickyView offset={{ closed: 0, opened: 0 }}>
      <View
        className="flex-row items-end gap-2 px-4 pt-2.5 bg-screen border-t border-border"
        style={{ paddingBottom }}
      >
        <View className="flex-1 bg-card border border-border rounded-[18px] px-3.5 min-h-[88px] justify-start">
          <TextInput
            style={styles.input}
            value={value}
            onChangeText={onChangeText}
            placeholder="Message the agent…"
            placeholderTextColor={theme.colors.text.muted}
            multiline
            textAlignVertical="top"
          />
        </View>
        {running && onStop ? (
          <Pressable
            className="w-11 h-11 rounded-full items-center justify-center"
            style={({ pressed }) => ({
              backgroundColor: "rgba(255,255,255,0.08)",
              opacity: pressed ? 0.7 : 1,
            })}
            onPress={onStop}
          >
            <Square size={16} color={theme.colors.text.primary} />
          </Pressable>
        ) : (
          <Pressable
            className="w-11 h-11 rounded-full items-center justify-center"
            style={({ pressed }) => ({
              backgroundColor: canSend ? theme.colors.text.primary : "rgba(255,255,255,0.08)",
              opacity: pressed && canSend ? 0.8 : 1,
            })}
            onPress={onSend}
            disabled={!canSend}
          >
            <ArrowUp size={20} color={canSend ? theme.colors.text.dark : theme.colors.text.muted} />
          </Pressable>
        )}
      </View>
    </KeyboardStickyView>
  );
}

const styles = StyleSheet.create({
  input: {
    flex: 1,
    minHeight: 76,
    maxHeight: 160,
    paddingTop: 10,
    paddingBottom: 10,
    color: theme.colors.text.primary,
    fontSize: 15,
    lineHeight: 22,
    includeFontPadding: false,
  },
});
