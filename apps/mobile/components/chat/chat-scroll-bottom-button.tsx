import React, { memo } from "react";
import { View, Pressable } from "react-native";
import { ArrowDown } from "lucide-react-native";
import { theme } from "../../styles/theme";

interface ChatScrollBottomButtonProps {
  visible: boolean;
  onPress: () => void;
  hasInteraction?: boolean;
}

export const ChatScrollBottomButton = memo(function ChatScrollBottomButton({
  visible,
  onPress,
  hasInteraction = false,
}: ChatScrollBottomButtonProps) {
  if (!visible) return null;

  return (
    <View
      className="absolute left-0 right-0 items-center z-30 pointer-events-box-none"
      style={{ bottom: hasInteraction ? 160 : 96 }}
      pointerEvents="box-none"
    >
      <Pressable
        onPress={onPress}
        className="w-9 h-9 rounded-full bg-card-alt border border-border items-center justify-center shadow-lg shadow-black/80 active:bg-surfaceElevated"
        style={({ pressed }) => ({
          opacity: pressed ? 0.8 : 1,
          transform: [{ scale: pressed ? 0.94 : 1 }],
        })}
        hitSlop={10}
      >
        <ArrowDown size={17} color={theme.colors.text.primary} />
      </Pressable>
    </View>
  );
});
