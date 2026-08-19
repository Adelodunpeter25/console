import React, { useRef, useCallback } from "react";
import { View, Pressable, StyleSheet } from "react-native";
import {
  BottomSheetModal,
  BottomSheetBackdrop,
  BottomSheetView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";
import { Pencil, Trash2 } from "lucide-react-native";
import { theme } from "../../styles/theme";

export interface ActionSheetItem {
  key: string;
  label: string;
  icon: React.ReactNode;
  destructive?: boolean;
  onPress: () => void;
}

interface SessionActionSheetProps {
  children: (onLongPress: () => void) => React.ReactNode;
  items: ActionSheetItem[];
}

export function SessionActionSheet({ children, items }: SessionActionSheetProps) {
  const sheetRef = useRef<BottomSheetModal>(null);

  const open = useCallback(() => {
    sheetRef.current?.present();
  }, []);

  const close = useCallback(() => {
    sheetRef.current?.dismiss();
  }, []);

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
    <>
      {children(open)}

      <BottomSheetModal
        ref={sheetRef}
        enableDynamicSizing
        backdropComponent={renderBackdrop}
        handleIndicatorStyle={{ backgroundColor: theme.colors.text.muted, width: 36, height: 4 }}
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

          {/* Safe area bottom padding */}
          <View style={styles.bottomPad} />
        </BottomSheetView>
      </BottomSheetModal>
    </>
  );
}

const styles = StyleSheet.create({
  sheet: {
    backgroundColor: "#141518",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
  },
  container: {
    paddingHorizontal: 16,
    paddingTop: 8,
  },
  titleRow: {
    paddingVertical: 12,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: "rgba(255,255,255,0.08)",
    marginBottom: 4,
  },
  title: {
    fontSize: 13,
    color: "#71717a",
    fontWeight: "500",
    textAlign: "center",
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
    height: 24,
  },
});
