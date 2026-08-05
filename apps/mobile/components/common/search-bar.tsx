import React from "react";
import { View, TextInput, TouchableOpacity, StyleSheet } from "react-native";
import { Search, Plus } from "lucide-react-native";
import { theme } from "../../styles/theme";

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
    <View style={styles.container}>
      <View style={styles.inputWrapper}>
        <Search size={18} color={theme.colors.text.muted} />
        <TextInput
          style={styles.input}
          placeholder={placeholder}
          placeholderTextColor={theme.colors.text.muted}
          value={value}
          onChangeText={onChangeText}
          autoCapitalize="none"
          autoCorrect={false}
        />
      </View>
      <TouchableOpacity style={styles.composeButton} onPress={onComposePress} activeOpacity={0.7}>
        <Plus size={20} color={theme.colors.text.dark} />
      </TouchableOpacity>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingTop: 12,
    paddingBottom: 24,
    backgroundColor: "rgba(13, 13, 14, 0.95)",
    borderTopWidth: 1,
    borderTopColor: theme.colors.border,
  },
  inputWrapper: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: theme.colors.surface,
    borderColor: theme.colors.border,
    borderWidth: 1,
    borderRadius: theme.roundness.full,
    paddingHorizontal: 16,
    height: 48,
    marginRight: 12,
  },
  input: {
    flex: 1,
    marginLeft: 10,
    color: theme.colors.text.primary,
    fontSize: 14,
    height: "100%",
  },
  composeButton: {
    width: 48,
    height: 48,
    borderRadius: theme.roundness.full,
    backgroundColor: theme.colors.text.primary,
    alignItems: "center",
    justifyContent: "center",
  },
});
