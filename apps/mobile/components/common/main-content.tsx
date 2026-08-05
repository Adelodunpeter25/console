import React from "react";
import { AppShell } from "./app-shell";
import { HomeScreen, ChatScreen, SettingsScreen } from "../../screens";
import { useAppStore } from "../../stores";

export function MainContent() {
  const activeTab = useAppStore((state) => state.activeTab);

  return (
    <AppShell>
      {activeTab === "home" ? (
        <HomeScreen />
      ) : activeTab === "chat" ? (
        <ChatScreen />
      ) : (
        <SettingsScreen />
      )}
    </AppShell>
  );
}
