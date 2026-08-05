import React, { useMemo, useState } from "react";
import { Alert, Text, View, TouchableOpacity, ScrollView } from "react-native";
import { GitBranch } from "lucide-react-native";
import { useCreateSession, useProjects, useSessions } from "@console/api";
import { SessionHeader } from "@console/types";
import { ScreenHeader } from "../../components/layout/screen-header";
import { SearchBar } from "../../components/common/search-bar";
import { useAppStore } from "../../stores";
import { formatRelativeTime } from "../../utils/time";
import { theme } from "../../styles/theme";

interface GroupedSection {
  projectId: string | null;
  projectName: string;
  data: SessionHeader[];
}

function getStatusStyle(status?: string): {
  label: string;
  color: string;
  bgColor: string;
} {
  switch (status) {
    case "working":
      return {
        label: "Running",
        color: theme.colors.status.running,
        bgColor: theme.colors.status.runningBg,
      };
    case "done":
      return {
        label: "Ready",
        color: theme.colors.status.ready,
        bgColor: theme.colors.status.readyBg,
      };
    case "needs_attention":
      return {
        label: "Attention",
        color: theme.colors.status.attention,
        bgColor: theme.colors.status.attentionBg,
      };
    default:
      return {
        label: "Idle",
        color: theme.colors.status.idle,
        bgColor: theme.colors.status.idleBg,
      };
  }
}

function shortRelativeTime(dateInput?: number): string {
  const full = formatRelativeTime(dateInput);
  return full.replace(" ago", "").replace("just now", "now");
}

export function HomeScreen() {
  const { data: projects = [] } = useProjects();
  const { data: sessions = [] } = useSessions();
  const createSession = useCreateSession();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
  const [searchQuery, setSearchQuery] = useState("");

  // Filter sessions by search query
  const filteredSessions = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return sessions;
    return sessions.filter((s) => (s.title || "").toLowerCase().includes(q));
  }, [sessions, searchQuery]);

  // Group filtered sessions by project
  const sections = useMemo<GroupedSection[]>(() => {
    const byProject = new Map<string | null, SessionHeader[]>();
    for (const session of filteredSessions) {
      const key = session.projectId ?? null;
      const list = byProject.get(key) ?? [];
      list.push(session);
      byProject.set(key, list);
    }

    const result: GroupedSection[] = [];
    for (const [projectId, list] of byProject) {
      const project = projects.find((p) => p.id === projectId);
      result.push({
        projectId,
        projectName: project ? project.name.toUpperCase() : "DRAFT",
        data: list.sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0)),
      });
    }
    return result.sort((a, b) => a.projectName.localeCompare(b.projectName));
  }, [filteredSessions, projects]);

  const openSession = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    setActiveTab("chat");
  };

  const composeSession = async () => {
    if (createSession.isPending) return;

    // Prefer the first configured project. The server accepts an empty cwd and
    // falls back to its own working directory when no project exists yet.
    const project = projects[0];
    try {
      const session = await createSession.mutateAsync({
        cwd: project?.path ?? "",
        ...(project ? { projectId: project.id } : {}),
        title: "New Chat",
      });
      setSelectedSessionId(session.id);
      setActiveTab("chat");
    } catch (error) {
      console.error("Failed to create session:", error);
      Alert.alert("Unable to start chat", "Check the backend connection and try again.");
    }
  };

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader title="Console" showSettings onSettingsPress={() => setActiveTab("settings")} />

      <ScrollView
        className="flex-1"
        contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 32 }}
        keyboardShouldPersistTaps="handled"
        showsVerticalScrollIndicator={false}
      >
        {sections.length === 0 ? (
          <View className="items-center justify-center py-20">
            <Text className="text-foreground-secondary text-sm">
              {searchQuery ? "No matching sessions" : "No sessions yet"}
            </Text>
          </View>
        ) : (
          sections.map((section) => (
            <View key={section.projectId ?? "draft"} className="mb-5">
              {/* Section Header */}
              <View className="flex-row justify-between items-center mb-2 px-1">
                <Text className="text-xs font-bold text-foreground-secondary tracking-widest">
                  📁 {section.projectName}
                </Text>
                <Text className="text-xs text-foreground-secondary opacity-60">
                  {section.data.length} more
                </Text>
              </View>

              {/* Session Cards grouped into one card */}
              <View className="bg-card border border-border rounded-2xl overflow-hidden">
                {section.data.map((session, index) => {
                  const project = projects.find((p) => p.id === session.projectId);
                  const status = getStatusStyle(session.status);
                  const projectName = project?.name ?? "DeskMini";
                  const isLast = index === section.data.length - 1;

                  return (
                    <TouchableOpacity
                      key={session.id}
                      className={`flex-row items-center px-4 py-3.5 ${!isLast ? "border-b border-border/40" : ""}`}
                      onPress={() => openSession(session.id)}
                      activeOpacity={0.65}
                    >
                      {/* Avatar icon with status colour */}
                      <View
                        className="w-9 h-9 rounded-xl items-center justify-center mr-3"
                        style={{ backgroundColor: status.bgColor }}
                      >
                        <GitBranch size={16} color={status.color} />
                      </View>

                      {/* Title + project/branch */}
                      <View className="flex-1 mr-2">
                        <Text
                          className="text-sm font-semibold text-foreground mb-0.5"
                          numberOfLines={1}
                        >
                          {session.title || "Untitled Session"}
                        </Text>
                        <View className="flex-row items-center gap-1">
                          <Text className="text-xs text-foreground-secondary">{projectName}</Text>
                          <Text className="text-xs text-foreground-secondary">•</Text>
                          <Text className="text-xs text-foreground-secondary">main</Text>
                        </View>
                      </View>

                      {/* Status badge + time */}
                      <View className="items-end gap-1.5">
                        <View
                          className="px-2 py-0.5 rounded-full"
                          style={{ backgroundColor: status.bgColor }}
                        >
                          <Text
                            className="text-[9px] font-bold tracking-wide"
                            style={{ color: status.color }}
                          >
                            {status.label}
                          </Text>
                        </View>
                        <Text className="text-[10px] text-foreground-secondary">
                          {shortRelativeTime(session.updatedAt ?? session.createdAt)}
                        </Text>
                      </View>
                    </TouchableOpacity>
                  );
                })}
              </View>
            </View>
          ))
        )}
      </ScrollView>

      {/* Keep the composer outside the list so it stays anchored to the resized window. */}
      <SearchBar
        value={searchQuery}
        onChangeText={setSearchQuery}
        onComposePress={composeSession}
        disabled={createSession.isPending}
      />
    </View>
  );
}

export default HomeScreen;
