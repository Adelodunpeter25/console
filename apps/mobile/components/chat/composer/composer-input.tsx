import React, { useState } from "react";
import { View, TextInput, Pressable, StyleSheet } from "react-native";
import { ArrowUp, Square, Plus } from "lucide-react-native";
import { theme } from "@/styles/theme";

interface ComposerInputProps {
  value: string;
  onChangeText: (text: string) => void;
  selection: { start: number; end: number };
  onSelectionChange: (selection: { start: number; end: number }) => void;
  inputRef: React.RefObject<TextInput | null>;
  canSend: boolean;
  running?: boolean;
  onSend: () => void;
  onStop?: () => void;
  onPickImage: () => void;
}

export function ComposerInput({
  value,
  onChangeText,
  selection,
  onSelectionChange,
  inputRef,
  canSend,
  running,
  onSend,
  onStop,
  onPickImage,
}: ComposerInputProps) {
  const [isMultiline, setIsMultiline] = useState(false);

  return (
    <View
      className={`flex-row ${
        isMultiline ? "items-end pb-1" : "items-center"
      } bg-card border border-border/80 pl-2 pr-1.5 py-1 min-h-[48px] ${
        isMultiline ? "rounded-2xl" : "rounded-full"
      }`}
    >
      <Pressable
        onPress={onPickImage}
        className="w-8 h-8 rounded-full items-center justify-center mr-1"
        style={({ pressed }) => ({
          backgroundColor: pressed ? "rgba(255,255,255,0.08)" : "transparent",
          opacity: pressed ? 0.7 : 0.9,
        })}
        hitSlop={6}
      >
        <Plus size={21} color={theme.colors.text.secondary} />
      </Pressable>

      <TextInput
        ref={inputRef}
        style={styles.input}
        value={value}
        onChangeText={onChangeText}
        placeholder="Ask anything…"
        placeholderTextColor={theme.colors.text.muted}
        multiline
        textAlignVertical={isMultiline ? "top" : "center"}
        selection={selection}
        onSelectionChange={(e) => onSelectionChange(e.nativeEvent.selection)}
        onContentSizeChange={(e) => {
          const height = e.nativeEvent.contentSize.height;
          setIsMultiline(height > 36 || value.includes("\n"));
        }}
      />

      {running && onStop ? (
        <Pressable
          className="w-8 h-8 rounded-full items-center justify-center ml-1.5"
          style={({ pressed }) => ({
            backgroundColor: theme.colors.text.primary,
            opacity: pressed ? 0.7 : 1,
          })}
          onPress={onStop}
          accessibilityLabel="Stop"
        >
          <Square size={12} color="#000000" fill="#000000" />
        </Pressable>
      ) : (
        <Pressable
          className="w-8 h-8 rounded-full items-center justify-center ml-1.5"
          style={({ pressed }) => ({
            backgroundColor: canSend ? theme.colors.text.primary : "rgba(255,255,255,0.08)",
            opacity: pressed && canSend ? 0.8 : 1,
          })}
          onPress={onSend}
          disabled={!canSend}
        >
          <ArrowUp size={16} color={canSend ? theme.colors.text.dark : theme.colors.text.muted} />
        </Pressable>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  input: {
    flex: 1,
    maxHeight: 120,
    paddingTop: 8,
    paddingBottom: 8,
    color: theme.colors.text.primary,
    fontSize: 14,
    lineHeight: 19,
    includeFontPadding: false,
  },
});
