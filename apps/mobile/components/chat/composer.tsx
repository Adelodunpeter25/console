import React from "react";
import { View, Text, TextInput, TouchableOpacity, ActivityIndicator } from "react-native";

interface ComposerProps {
  value: string;
  onChangeText: (text: string) => void;
  onSend: () => void;
  onStop?: () => void;
  running?: boolean;
}

/** Chat input row. Shows a stop button while a run is in progress. */
export function Composer({ value, onChangeText, onSend, onStop, running }: ComposerProps) {
  return (
    <View className="flex-row gap-2.5 px-4 py-3 bg-screen border-t border-border items-end">
      <TextInput
        className="flex-1 min-h-11 max-h-28 bg-card border border-border rounded-2xl px-4 py-2.5 text-foreground text-sm"
        value={value}
        onChangeText={onChangeText}
        placeholder="Ask agent to write code..."
        placeholderTextColor="#71717a"
        multiline
      />
      {running && onStop ? (
        <TouchableOpacity
          className="w-11 h-11 bg-card border border-border rounded-full items-center justify-center"
          onPress={onStop}
          activeOpacity={0.7}
        >
          <View className="w-3.5 h-3.5 rounded-sm bg-foreground" />
        </TouchableOpacity>
      ) : (
        <TouchableOpacity
          className={`w-11 h-11 bg-foreground rounded-full items-center justify-center ${
            !value.trim() ? "opacity-30" : ""
          }`}
          onPress={onSend}
          disabled={!value.trim() || Boolean(running)}
        >
          {running ? (
            <ActivityIndicator size="small" color="#000000" />
          ) : (
            <Text className="text-black text-sm font-bold">↑</Text>
          )}
        </TouchableOpacity>
      )}
    </View>
  );
}
