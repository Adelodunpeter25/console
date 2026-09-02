import React from "react";
import { View } from "react-native";
import { AppShell } from "./app-shell";
import { HomeScreen, ChatScreen, SettingsScreen, TerminalScreen, SubagentsScreen, SubagentDetailsScreen } from "@/screens";
import { FilesScreen } from "@/screens/files/files-screen";
import { app$ } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";

export function MainContent() {
  const activeTab = useValue(app$.activeTab);

  return (
    <AppShell>
      {/*
        Home stays mounted across tab switches so the session list, scroll
        position, and rendered component tree aren't destroyed and rebuilt
        on every chat → home round-trip. Hidden via display:none when
        inactive — hooks keep running (cheap with the 5-min query cache) but
        no layout work or visual flash. Chat and Settings are conditionally
        rendered because they carry heavier per-screen side effects.
      */}
      <View style={{ flex: 1, display: activeTab === "home" ? "flex" : "none" }}>
        <HomeScreen />
      </View>
      {activeTab === "chat" ? <ChatScreen /> : null}
      {activeTab === "settings" ? <SettingsScreen /> : null}
      {/* Conditional like chat: the native terminal surface carries heavy side effects. */}
      {activeTab === "terminal" ? <TerminalScreen /> : null}
      {activeTab === "files" ? <FilesScreen /> : null}
      {activeTab === "subagents" ? <SubagentsScreen /> : null}
      {activeTab === "subagent-details" ? <SubagentDetailsScreen /> : null}
    </AppShell>
  );
}
