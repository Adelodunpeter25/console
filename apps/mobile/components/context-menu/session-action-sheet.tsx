import React, { useRef, useCallback } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import {
  BottomSheetModal,
  BottomSheetBackdrop,
  BottomSheetView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";
import { theme } from "../../styles/theme";

export interface ActionSheetItem {
  key: string;
  label: string;
  icon: React.ReactNode;
  destructive?: boolean;
  onPress: () => void;
}

interface SessionActionSheetProps {
  items: ActionSheetItem[];
}

export interface SessionActionSheetHandle {
  open: () => void;
}

/** Single shared action sheet — render once at page level, call open() imperatively. */
export const SessionActionSheet = React.forwardRef<
  SessionActionSheetHandle,
  SessionActionSheetProps
>(function SessionActionSheet({ items }, ref) {
  const sheetRef = useRef<BottomSheetModal>(null);

  React.useImperativeHandle(ref, () => ({
    open: () => sheetRef.current?.present(),
  }));

  const close = useCallback(() => sheetRef.current?.dismiss(), []);

  const renderBackdrop = useCallback(
    (props: BottomSheetBackdropProps) => (
      <BottomSheetBackdrop
        {...props}
        disappearsOnIndex={-1}
        appearsOnIndex={0}
        opacity={0.55}
        pressBehavior="close"
      />
    ),
    [],
  );

  return (
    <BottomSheetModal
      ref={sheetRef}
      enableDynamicSizing
      backdropComponent={renderBackdrop}
      handleStyle={{ display: "none" }}
      backgroundStyle={styles.sheet}
      enablePanDownToClose
    >
      <BottomSheetView style={styles.container}>
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
                close();
                setTimeout(item.onPress, 200);
              }}
            >
              <View style={styles.iconWrap}>{item.icon}</View>
              <Text style={[styles.label, item.destructive && styles.labelDestructive]}>
                {item.label}
              </Text>
            </Pressable>
          );
        })}
        <View style={styles.bottomPad} />
      </BottomSheetView>
    </BottomSheetModal>
  );
});

const styles = StyleSheet.create({
  sheet: {
    backgroundColor: "#141518",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
  },
  container: {
    paddingHorizontal: 16,
    paddingTop: 16,
  },
  item: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 16,
    gap: 14,
  },
  itemBorder: {
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: "rgba(255,255,255,0.08)",
  },
  itemPressed: {
    opacity: 0.6,
  },
  iconWrap: {
    width: 24,
    alignItems: "center",
  },
  label: {
    fontSize: 16,
    color: "#ffffff",
    fontWeight: "500",
  },
  labelDestructive: {
    color: "#f87171",
  },
  bottomPad: {
    height: 44,
  },
});
