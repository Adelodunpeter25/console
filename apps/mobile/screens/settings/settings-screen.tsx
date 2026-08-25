import React, { useState, useEffect } from "react";
import { Text, View, Pressable, BackHandler } from "react-native";
import { ChevronRight, Wifi, User, Folder, Trash2 } from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { useProjectStore } from "@/stores";
import { useAuth } from "@/hooks";
import { EnvironmentsSettings } from "./environments-settings";
import { AccountSettings } from "./account-settings";
import { ProjectsSettings } from "./projects-settings";
import { DeletedChatsSettings } from "./deleted-chats-settings";
import { theme } from "@/styles/theme";
import { app$, setActiveTab, setPendingConnectionSection } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";

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
  const pendingConnectionSection = useValue(app$.pendingConnectionSection);
  const [section, setSection] = useState<SettingsSection | null>(null);

  // If the environment switcher's "Add environment" button sent us here, jump
  // straight into the Connection sub-screen and clear the flag.
  useEffect(() => {
    if (pendingConnectionSection) {
      setSection("connection");
      setPendingConnectionSection(false);
    }
  }, [pendingConnectionSection, setPendingConnectionSection]);

  useEffect(() => {
    const onBackPress = () => {
      if (section !== null) {
        setSection(null);
        return true;
      }
      setActiveTab("home");
      return true;
    };
    const sub = BackHandler.addEventListener("hardwareBackPress", onBackPress);
    return () => sub.remove();
  }, [section, setActiveTab]);
  const backendUrl = useValue(app$.backendUrl);
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
      <View style={{ flex: 1, backgroundColor: theme.colors.background }}>
        {section === "projects" ? (
          <ProjectsSettings onBack={() => setSection(null)} />
        ) : section === "deleted-chats" ? (
          <DeletedChatsSettings onBack={() => setSection(null)} />
        ) : (
          <>
            {section === "connection" ? <EnvironmentsSettings onBack={() => setSection(null)} /> : null}
            {section === "account" ? (
              <>
                <ScreenHeader title={meta.title} onBack={() => setSection(null)} />
                <AccountSettings />
              </>
            ) : null}
          </>
        )}
      </View>
    );
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.background }}>
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
              <ChevronRight size={20} color={theme.colors.text.muted} />
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

export default SettingsScreen;
