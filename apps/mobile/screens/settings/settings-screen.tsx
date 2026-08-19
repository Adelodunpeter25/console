import React, { useState } from "react";
import { Text, View, Pressable } from "react-native";
import { ChevronRight, Wifi, User, Folder, Trash2 } from "lucide-react-native";
import { ScreenHeader } from "../../components/layout/screen-header";
import { useAppStore, useProjectStore } from "../../stores";
import { useServerConnection } from "../../hooks";
import { useAuth } from "../../hooks";
import { ConnectionSettings } from "./connection-settings";
import { AccountSettings } from "./account-settings";
import { ProjectsSettings } from "./projects-settings";
import { DeletedChatsSettings } from "./deleted-chats-settings";

type SettingsSection = "connection" | "account" | "projects" | "deleted-chats";

const SECTION_META: Record<
  SettingsSection,
  { title: string; icon: React.ComponentType<{ size?: number; color?: string }> }
> = {
  connection: { title: "Connection", icon: Wifi },
  account: { title: "Account", icon: User },
  projects: { title: "Projects", icon: Folder },
  "deleted-chats": { title: "Deleted Chats", icon: Trash2 },
};

export function SettingsScreen() {
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const [section, setSection] = useState<SettingsSection | null>(null);

  const { backendUrl } = useServerConnection();
  const auth = useAuth();
  const projects = useProjectStore((state) => state.projects);
  const deletedSessions = useProjectStore((state) => state.deletedSessions);

  // Summary text under each landing row.
  const summary: Record<SettingsSection, string> = {
    connection: backendUrl ? "Connected" : "Not connected",
    account:
      auth.status && Object.values(auth.status).some((s) => s.loggedIn)
        ? "Signed in"
        : "No providers connected",
    projects: `${projects.length} project folder${projects.length === 1 ? "" : "s"}`,
    "deleted-chats": `${deletedSessions.length} deleted chat${deletedSessions.length === 1 ? "" : "s"}`,
  };

  if (section) {
    const meta = SECTION_META[section];
    return (
      <View style={{ flex: 1, backgroundColor: "#0a0a0b" }}>
        {section === "projects" ? (
          <ProjectsSettings onBack={() => setSection(null)} />
        ) : (
          <>
            <ScreenHeader title={meta.title} onBack={() => setSection(null)} />
            {section === "connection" ? <ConnectionSettings /> : null}
            {section === "account" ? <AccountSettings /> : null}
            {section === "deleted-chats" ? <DeletedChatsSettings /> : null}
          </>
        )}
      </View>
    );
  }

  return (
    <View style={{ flex: 1, backgroundColor: "#0a0a0b" }}>
      <ScreenHeader title="Settings" onBack={() => setActiveTab("home")} />
      <View style={{ flex: 1, paddingHorizontal: 16, paddingTop: 8 }}>
        {(Object.keys(SECTION_META) as SettingsSection[]).map((key) => {
          const meta = SECTION_META[key];
          const Icon = meta.icon;
          return (
            <Pressable
              key={key}
              onPress={() => setSection(key)}
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
              className="flex-row items-center bg-card border border-border rounded-2xl px-4 py-4 mb-3"
            >
              <View className="w-10 h-10 rounded-xl bg-foreground/10 items-center justify-center mr-3">
                <Icon size={18} color="#ffffff" />
              </View>
              <View className="flex-1">
                <Text className="text-base font-semibold text-foreground">{meta.title}</Text>
                <Text className="text-xs text-foreground-secondary mt-0.5">{summary[key]}</Text>
              </View>
              <ChevronRight size={20} color="#71717a" />
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

export default SettingsScreen;
