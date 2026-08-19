import React, { useRef, useCallback } from "react";
import {
  View,
  Text,
  Modal,
  Pressable,
  StyleSheet,
  Animated,
  Dimensions,
} from "react-native";

export interface ContextMenuItem {
  key: string;
  label: string;
  icon?: React.ReactNode;
  /** When true, renders the item in red/destructive style */
  destructive?: boolean;
  onPress: () => void;
}

interface BaseContextMenuProps {
  visible: boolean;
  onClose: () => void;
  /** Pixel coords of the long-pressed element, used for anchoring */
  anchor: { x: number; y: number; width: number; height: number } | null;
  items: ContextMenuItem[];
}

const MENU_WIDTH = 200;
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

  // Position: prefer below the anchor, flip up if too close to bottom
  const menuHeight = items.length * ITEM_HEIGHT;
  let top = anchor ? anchor.y + anchor.height + 6 : 200;
  let left = anchor ? anchor.x : 16;

  if (top + menuHeight > SCREEN.height - 40) {
    top = anchor ? anchor.y - menuHeight - 6 : 200;
  }
  if (left + MENU_WIDTH > SCREEN.width - 12) {
    left = SCREEN.width - MENU_WIDTH - 12;
  }
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
      {/* Scrim — tap anywhere to dismiss */}
      <Pressable style={StyleSheet.absoluteFill} onPress={handleClose}>
        <View style={[StyleSheet.absoluteFill, styles.scrim]} />
      </Pressable>

      {/* Menu card */}
      <Animated.View
        style={[
          styles.card,
          { top, left, opacity, transform: [{ scale }] },
        ]}
        pointerEvents="box-none"
      >
        {items.map((item, index) => {
          const isLast = index === items.length - 1;
          return (
            <Pressable
              key={item.key}
              style={({ pressed }) => [
                styles.item,
                !isLast && styles.itemBorder,
                pressed && styles.itemPressed,
              ]}
              onPress={() => {
                handleClose();
                // Small delay so the menu closes before the action fires
                setTimeout(item.onPress, 150);
              }}
            >
              {item.icon ? (
                <View style={styles.iconWrap}>{item.icon}</View>
              ) : null}
              <Text
                style={[
                  styles.label,
                  item.destructive && styles.labelDestructive,
                ]}
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

const styles = StyleSheet.create({
  scrim: {
    backgroundColor: "rgba(0,0,0,0.45)",
  },
  card: {
    position: "absolute",
    width: MENU_WIDTH,
    backgroundColor: "#1c1c1e",
    borderRadius: 14,
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
    overflow: "hidden",
    shadowColor: "#000",
    shadowOpacity: 0.5,
    shadowRadius: 20,
    shadowOffset: { width: 0, height: 8 },
    elevation: 20,
  },
  item: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    height: ITEM_HEIGHT,
  },
  itemBorder: {
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: "rgba(255,255,255,0.08)",
  },
  itemPressed: {
    backgroundColor: "rgba(255,255,255,0.06)",
  },
  iconWrap: {
    marginRight: 10,
  },
  label: {
    fontSize: 15,
    color: "#ffffff",
    fontWeight: "500",
  },
  labelDestructive: {
    color: "#f87171",
  },
});
