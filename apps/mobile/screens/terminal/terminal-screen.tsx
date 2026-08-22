import React, { useCallback, useEffect } from "react";
import { View, BackHandler } from "react-native";
import { ScreenHeader } from "../../components/layout/screen-header";
import { useAppStore } from "../../stores";

/**
 * Terminal screen. Not yet registered as a tab — wiring lands with the rest
 * of Phase 4.
 */
export function TerminalScreen() {
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  const handleBack = useCallback(() => {
    setActiveTab("home");
  }, [setActiveTab]);

  useEffect(() => {
    const onBackPress = () => {
      handleBack();
      return true;
    };
    const sub = BackHandler.addEventListener("hardwareBackPress", onBackPress);
    return () => sub.remove();
  }, [handleBack]);

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader title="Terminal" onBack={handleBack} />

      <View className="flex-1">{/* Terminal surface lands here (Phase 4). */}</View>
    </View>
  );
}
