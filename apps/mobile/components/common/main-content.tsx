import React from "react";
import { SafeAreaView } from "react-native-safe-area-context";
import { HomeScreen, ChatScreen, SettingsScreen } from "../../screens";
import { useAppStore } from "../../stores";

export function MainContent() {
  const activeTab = useAppStore((state) => state.activeTab);

  return (
    <SafeAreaView className="flex-1 bg-[#0d0d0e]" edges={["top", "left", "right"]}>
      {activeTab === "home" ? (
        <HomeScreen />
      ) : activeTab === "chat" ? (
        <ChatScreen />
      ) : (
        <SettingsScreen />
      )}
    </SafeAreaView>
  );
}
