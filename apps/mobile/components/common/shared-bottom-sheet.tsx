import React, { forwardRef, useCallback, useMemo } from "react";
import { View, Text, StyleSheet } from "react-native";
import {
  BottomSheetModal,
  BottomSheetBackdrop,
  BottomSheetView,
  type BottomSheetBackdropProps,
} from "@gorhom/bottom-sheet";
import { theme } from "../../styles/theme";

interface SharedBottomSheetProps {
  title?: string;
  snapPoints?: (string | number)[];
  children: React.ReactNode;
}

export const SharedBottomSheet = forwardRef<BottomSheetModal, SharedBottomSheetProps>(
  function SharedBottomSheet({ title, snapPoints, children }, ref) {
    const defaultSnapPoints = useMemo(() => ["55%", "90%"], []);
    const snapPointsKey = Array.isArray(snapPoints) ? snapPoints.join(",") : "";
    const memoizedSnapPoints = useMemo(
      () => (snapPoints && snapPoints.length > 0 ? snapPoints : defaultSnapPoints),
      [snapPointsKey, defaultSnapPoints],
    );

    const renderBackdrop = useCallback(
      (props: BottomSheetBackdropProps) => (
        <BottomSheetBackdrop
          {...props}
          disappearsOnIndex={-1}
          appearsOnIndex={0}
          opacity={0.65}
          pressBehavior="close"
        />
      ),
      [],
    );

    return (
      <BottomSheetModal
        ref={ref}
        index={0}
        snapPoints={memoizedSnapPoints}
        enableDynamicSizing={false}
        backdropComponent={renderBackdrop}
        handleIndicatorStyle={{ backgroundColor: theme.colors.text.muted, width: 36, height: 4 }}
        backgroundStyle={{
          backgroundColor: "#141518",
          borderWidth: 1,
          borderColor: "rgba(255,255,255,0.08)",
        }}
        enablePanDownToClose
        keyboardBehavior="interactive"
        keyboardBlurBehavior="restore"
        android_keyboardInputMode="adjustResize"
      >
        <BottomSheetView style={styles.container}>
          {title ? (
            <View className="px-5 py-3 border-b border-border/40">
              <Text className="text-base font-semibold text-foreground">{title}</Text>
            </View>
          ) : null}
          <View style={styles.body}>{children}</View>
        </BottomSheetView>
      </BottomSheetModal>
    );
  },
);

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  body: {
    flex: 1,
    paddingHorizontal: 16,
    paddingTop: 12,
  },
});
