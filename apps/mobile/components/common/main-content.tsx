import React from "react";
import { SafeAreaView } from "react-native-safe-area-context";
import { useProjects } from "@console/api";
import { HomeScreen, ChatScreen, SettingsScreen } from "../../screens";
import { useAppStore } from "../../stores";

export function MainContent() {
  const activeTab = useAppStore((state) => state.activeTab);
  const { data: projects = [], refetch: refetchProjects } = useProjects();

  return (
    <SafeAreaView className="flex-1 bg-[#0d0d0e]" edges={["top", "left", "right"]}>
      {activeTab === "home" ? (
        <HomeScreen projects={projects} refetchProjects={refetchProjects} />
      ) : activeTab === "chat" ? (
        <ChatScreen />
      ) : (
        <SettingsScreen />
      )}
    </SafeAreaView>
  );
}
