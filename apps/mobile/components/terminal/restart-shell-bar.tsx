import React from "react";
import { Pressable, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useKeyboardState } from "react-native-keyboard-controller";

interface RestartShellBarProps {
  readonly onPress: () => void;
}

/** Shown when the PTY exited: respawn button pinned above the nav area. */
export function RestartShellBar({ onPress }: RestartShellBarProps) {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((s) => s.isVisible);

  return (
    <View
      className="flex-row items-center justify-center px-4 pt-3 border-t border-border-subtle bg-screen"
      style={{ paddingBottom: keyboardVisible ? 10 : insets.bottom + 10 }}
    >
      <Pressable
        className="px-4 py-2.5 rounded-full bg-foreground items-center justify-center"
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        onPress={onPress}
      >
        <Text className="text-xs font-semibold text-background">Restart shell</Text>
      </Pressable>
    </View>
  );
}
