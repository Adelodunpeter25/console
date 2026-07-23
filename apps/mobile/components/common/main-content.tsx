import React from "react";
import { SafeAreaView } from "react-native-safe-area-context";
import { useProjects } from "@console/api";
import { styles } from "../../styles/styles";
import { HomeScreen } from "./home-screen";
import { ChatScreen } from "../chat/chat-screen";

interface MainContentProps {
  activeTab: "home" | "chat";
  selectedProjectId: string | null;
  setSelectedProjectId: (id: string | null) => void;
  selectedSessionId: string | null;
  setSelectedSessionId: (id: string | null) => void;
  setActiveTab: (tab: "home" | "chat") => void;
  backendUrl: string;
}

export function MainContent({
  activeTab,
  selectedProjectId,
  setSelectedProjectId,
  selectedSessionId,
  setSelectedSessionId,
  setActiveTab,
  backendUrl,
}: MainContentProps) {
  const { data: projects = [], refetch: refetchProjects } = useProjects();

  return (
    <SafeAreaView style={styles.safeArea} edges={["top", "left", "right"]}>
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
      ) : (
        <ChatScreen
          projectId={selectedProjectId}
          sessionId={selectedSessionId}
          backendUrl={backendUrl}
        />
      )}
    </SafeAreaView>
  );
}
