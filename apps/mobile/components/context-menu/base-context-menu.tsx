import React, { useRef, useCallback } from "react";
import {
  View,
  Text,
  Modal,
  Pressable,
  Animated,
  Dimensions,
} from "react-native";
import { BlurView } from "expo-blur";

export interface ContextMenuItem {
  key: string;
  label: string;
  icon?: React.ReactNode;
  /** Renders the item in destructive (red) style */
  destructive?: boolean;
  onPress: () => void;
}

interface BaseContextMenuProps {
  visible: boolean;
  onClose: () => void;
  /** Pixel coords of the long-pressed element for anchoring */
  anchor: { x: number; y: number; width: number; height: number } | null;
  items: ContextMenuItem[];
}

const MENU_WIDTH = 210;
const ITEM_HEIGHT = 46;
const SCREEN = Dimensions.get("window");

export function BaseContextMenu({
  visible,
  onClose,
  anchor,
  items,
}: BaseContextMenuProps) {
  const opacity = useRef(new Animated.Value(0)).current;
  const scale = useRef(new Animated.Value(0.92)).current;

  const onShow = useCallback(() => {
    Animated.parallel([
      Animated.spring(opacity, { toValue: 1, useNativeDriver: true, damping: 20, stiffness: 300 }),
      Animated.spring(scale, { toValue: 1, useNativeDriver: true, damping: 20, stiffness: 300 }),
    ]).start();
  }, [opacity, scale]);

  const handleClose = useCallback(() => {
    Animated.parallel([
      Animated.timing(opacity, { toValue: 0, duration: 120, useNativeDriver: true }),
      Animated.timing(scale, { toValue: 0.92, duration: 120, useNativeDriver: true }),
    ]).start(() => onClose());
  }, [opacity, scale, onClose]);

  const menuHeight = items.length * ITEM_HEIGHT;
  let top = anchor ? anchor.y + anchor.height + 6 : 200;
  let left = anchor ? anchor.x : 16;

  if (top + menuHeight > SCREEN.height - 40) {
    top = anchor ? anchor.y - menuHeight - 6 : 200;
  }
  if (left + MENU_WIDTH > SCREEN.width - 12) left = SCREEN.width - MENU_WIDTH - 12;
  if (left < 12) left = 12;

  return (
    <Modal
      visible={visible}
      transparent
      animationType="none"
      statusBarTranslucent
      onShow={onShow}
      onRequestClose={handleClose}
    >
      {/* Blur scrim */}
      <Pressable className="absolute inset-0" onPress={handleClose}>
        <BlurView intensity={14} tint="dark" className="absolute inset-0" />
      </Pressable>

      {/* Menu card */}
      <Animated.View
        style={{ position: "absolute", width: MENU_WIDTH, top, left, opacity, transform: [{ scale }] }}
        className="bg-[#1c1c1e] rounded-[14px] border border-white/[0.08] overflow-hidden shadow-2xl"
        pointerEvents="box-none"
      >
        {items.map((item, index) => {
          const isLast = index === items.length - 1;
          return (
            <Pressable
              key={item.key}
              className={`flex-row items-center px-4 h-[46px] ${
                !isLast ? "border-b border-white/[0.08]" : ""
              }`}
              style={({ pressed }) => ({
                backgroundColor: pressed ? "rgba(255, 255, 255, 0.06)" : "transparent",
              })}
              onPress={() => {
                handleClose();
                setTimeout(item.onPress, 150);
              }}
            >
              {item.icon ? <View className="mr-2.5">{item.icon}</View> : null}
              <Text
                className={`text-[15px] font-medium ${
                  item.destructive ? "text-red-400" : "text-foreground"
                }`}
              >
                {item.label}
              </Text>
            </Pressable>
          );
        })}
      </Animated.View>
    </Modal>
  );
}
