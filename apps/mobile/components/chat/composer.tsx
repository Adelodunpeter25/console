import React from "react";
import { View, TextInput, Pressable } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
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
  const canSend = value.trim().length > 0;
  // Same bottom-inset pattern as SearchBar: clear of the home indicator when
  // idle; flush above the keyboard once the window has resized.
  const paddingBottom = Math.max(insets.bottom, 8) + 4;

  return (
    <View
      className="flex-row items-end gap-2 px-4 pt-2.5 bg-screen border-t border-border"
      style={{ paddingBottom }}
    >
      <View className="flex-1 flex-row items-end bg-card border border-border rounded-[22px] px-4 min-h-[48px]">
        <TextInput
          className="flex-1 max-h-[132px] py-2.5 text-foreground text-[15px] leading-[22px]"
          value={value}
          onChangeText={onChangeText}
          placeholder="Message the agent…"
          placeholderTextColor={theme.colors.text.muted}
          multiline
          textAlignVertical="center"
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
  );
}
