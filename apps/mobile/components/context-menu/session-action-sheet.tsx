import React, { useRef, useCallback } from "react";
import { View, Text, Pressable } from "react-native";
import {
  BottomSheetModal,
  BottomSheetBackdrop,
  BottomSheetView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";

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
      backgroundStyle={{
        backgroundColor: "#141518",
        borderWidth: 1,
        borderColor: "rgba(255, 255, 255, 0.08)",
      }}
      enablePanDownToClose
    >
      <BottomSheetView className="px-4 pt-4">
        {items.map((item, index) => {
          const isLast = index === items.length - 1;
          return (
            <Pressable
              key={item.key}
              className={`flex-row items-center py-4 gap-3.5 ${
                !isLast ? "border-b border-white/[0.08]" : ""
              }`}
              style={({ pressed }) => ({ opacity: pressed ? 0.6 : 1 })}
              onPress={() => {
                close();
                setTimeout(item.onPress, 200);
              }}
            >
              <View className="w-6 items-center">{item.icon}</View>
              <Text
                className={`text-base font-medium ${
                  item.destructive ? "text-red-400" : "text-foreground"
                }`}
              >
                {item.label}
              </Text>
            </Pressable>
          );
        })}
        <View className="h-11" />
      </BottomSheetView>
    </BottomSheetModal>
  );
});
