import React from "react";
import { Pressable, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { Keyboard, KeyboardOff } from "lucide-react-native";
import { useKeyboardState } from "react-native-keyboard-controller";

/** Extra keys sent verbatim to the PTY. */
const EXTRA_KEYS: { label: string; bytes: string }[] = [
  { label: "Esc", bytes: "\u001B" },
  { label: "Tab", bytes: "\t" },
  { label: "↑", bytes: "\u001B[A" },
  { label: "↓", bytes: "\u001B[B" },
  { label: "←", bytes: "\u001B[D" },
  { label: "→", bytes: "\u001B[C" },
  { label: "Ctrl-C", bytes: "\u0003" },
];

interface ExtraKeysBarProps {
  readonly onShowKeyboard: () => void;
  readonly onHideKeyboard: () => void;
  readonly onExtraKey: (bytes: string) => void;
}

/** PTY control strip that sits flush above the IME: keyboard toggle plus
 * extra keys the soft keyboard cannot produce (Esc, arrows, Ctrl-C, ...). */
export function ExtraKeysBar({ onShowKeyboard, onHideKeyboard, onExtraKey }: ExtraKeysBarProps) {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((s) => s.isVisible);

  return (
    <View
      className="flex-row items-center gap-1.5 px-2 pt-1.5 border-t border-border-subtle bg-screen"
      style={{ paddingBottom: keyboardVisible ? 6 : insets.bottom + 6 }}
    >
      <Pressable
        className="h-8 px-2.5 rounded-md bg-card border border-border items-center justify-center"
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        onPress={keyboardVisible ? onHideKeyboard : onShowKeyboard}
      >
        {keyboardVisible ? (
          <KeyboardOff size={15} color="#a1a1aa" />
        ) : (
          <Keyboard size={15} color="#a1a1aa" />
        )}
      </Pressable>
      {EXTRA_KEYS.map((key) => (
        <Pressable
          key={key.label}
          className="h-8 min-w-9 px-2 rounded-md bg-card border border-border items-center justify-center"
          style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
          onPress={() => onExtraKey(key.bytes)}
        >
          <Text className="text-xs font-mono text-foreground-secondary">{key.label}</Text>
        </Pressable>
      ))}
    </View>
  );
}
