import React from "react";
import { SafeAreaView } from "react-native-safe-area-context";
import { useProjects } from "@console/api";
import { HomeScreen } from "./home-screen";
import { ChatScreen } from "../chat/chat-screen";
import { SettingsScreen } from "../../screens/settings/settings-screen";

interface MainContentProps {
  activeTab: "home" | "chat" | "settings";
  selectedProjectId: string | null;
  setSelectedProjectId: (id: string | null) => void;
  selectedSessionId: string | null;
  setSelectedSessionId: (id: string | null) => void;
  setActiveTab: (tab: "home" | "chat" | "settings") => void;
  backendUrl: string;
  setBackendUrl: (url: string | null) => void;
}

export function MainContent({
  activeTab,
  selectedProjectId,
  setSelectedProjectId,
  selectedSessionId,
  setSelectedSessionId,
  setActiveTab,
  backendUrl,
  setBackendUrl,
}: MainContentProps) {
  const { data: projects = [], refetch: refetchProjects } = useProjects();

  return (
    <SafeAreaView className="flex-1 bg-[#0d0d0e]" edges={["top", "left", "right"]}>
      {activeTab === "home" ? (
        <HomeScreen
          projects={projects}
          refetchProjects={refetchProjects}
          selectedProjectId={selectedProjectId}
          setSelectedProjectId={setSelectedProjectId}
          selectedSessionId={selectedSessionId}
          setSelectedSessionId={setSelectedSessionId}
          setActiveTab={setActiveTab}
        />
      ) : activeTab === "chat" ? (
        <ChatScreen
          projectId={selectedProjectId}
          sessionId={selectedSessionId}
          backendUrl={backendUrl}
        />
      ) : (
        <SettingsScreen
          backendUrl={backendUrl}
          setBackendUrl={setBackendUrl}
          setActiveTab={setActiveTab}
        />
      )}
    </SafeAreaView>
  );
}
