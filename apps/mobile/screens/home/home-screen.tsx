import React, { useMemo, useState } from "react";
import { Text, View, TouchableOpacity, SectionList, TextInput } from "react-native";
import { Search, Plus, GitBranch, MessageSquare } from "lucide-react-native";
import { useProjects, useSessions } from "@console/api";
import { SessionHeader } from "@console/types";
import { ScreenHeader } from "../../components/common/screen-header";
import { useAppStore } from "../../stores";
import { formatRelativeTime } from "../../utils/time";

interface GroupedSection {
  projectId: string | null;
  projectName: string;
  data: SessionHeader[];
}

function sessionStatus(session: SessionHeader): {
  label: string;
  textColor: string;
  bgColor: string;
} {
  switch (session.status) {
    case "working":
      return { label: "Running", textColor: "text-orange-400", bgColor: "bg-orange-400/10" };
    case "done":
      return { label: "Ready", textColor: "text-emerald-400", bgColor: "bg-emerald-400/10" };
    case "needs_attention":
      return { label: "Needs Attention", textColor: "text-red-400", bgColor: "bg-red-400/10" };
    default:
      return { label: "Idle", textColor: "text-zinc-400", bgColor: "bg-zinc-500/10" };
  }
}

function shortRelativeTime(dateInput?: string | number): string {
  const full = formatRelativeTime(dateInput);
  return full.replace(" ago", "").replace("just now", "now");
}

export function HomeScreen() {
  const { data: projects = [] } = useProjects();
  const { data: sessions = [] } = useSessions();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
  const [searchQuery, setSearchQuery] = useState("");

  const sections = useMemo<GroupedSection[]>(() => {
    const byProject = new Map<string | null, SessionHeader[]>();
    for (const session of sessions) {
      const list = byProject.get(session.projectId ?? null) ?? [];
      list.push(session);
      byProject.set(session.projectId ?? null, list);
    }

    const result: GroupedSection[] = [];
    for (const [projectId, list] of byProject) {
      const project = projects.find((p) => p.id === projectId);
      const projectName = project ? project.name.toUpperCase() : "DRAFT";
      result.push({
        projectId,
        projectName,
        data: list.sort((a, b) => (b.updatedAt || b.createdAt) - (a.updatedAt || a.createdAt)),
      });
    }
    return result.sort((a, b) => a.projectName.localeCompare(b.projectName));
  }, [sessions, projects]);

  const openSession = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    setActiveTab("chat");
  };

  return (
    <View className="flex-1 bg-[#0d0d0e]" style={{ flex: 1 }}>
      <ScreenHeader
        title="Console"
        subtitle="Alpha"
        showSettings
        onSettingsPress={() => setActiveTab("settings")}
      />

      {/* Session List */}
      <SectionList
        sections={sections}
        keyExtractor={(item) => item.id}
        contentContainerStyle={{ paddingBottom: 96 }}
        stickySectionHeadersEnabled={false}
        renderSectionHeader={({ section }: { section: GroupedSection }) => (
          <View className="flex-row justify-between items-center px-4 py-2.5 bg-[#0d0d0e]">
            <Text className="text-xs font-bold text-zinc-500 uppercase tracking-wider">
              {section.projectName}
            </Text>
            <Text className="text-xs text-zinc-600">{section.data.length} more</Text>
          </View>
        )}
        renderItem={({ item: session }: { item: SessionHeader }) => {
          const project = projects.find((p) => p.id === session.projectId);
          const status = sessionStatus(session);
          const projectName = project ? project.name : "DeskMini";

          return (
            <TouchableOpacity
              className="flex-row items-center px-4 py-3.5 border-b border-white/5 active:bg-white/5"
              onPress={() => openSession(session.id)}
            >
              <View className="w-8 h-8 rounded-full bg-zinc-800 items-center justify-center mr-3">
                <MessageSquare size={16} color="#a1a1aa" />
              </View>
              <View className="flex-1 pr-3">
                <Text className="text-sm font-semibold text-white" numberOfLines={1}>
                  {session.title || "Untitled Session"}
                </Text>
                <View className="flex-row items-center mt-0.5">
                  <Text className="text-xs text-zinc-500 mr-1.5">{projectName}</Text>
                  <GitBranch size={10} color="#71717a" />
                  <Text className="text-xs text-zinc-500 ml-1">main</Text>
                </View>
              </View>
              <View className="items-end">
                <View className={`px-2 py-0.5 rounded-full ${status.bgColor}`}>
                  <Text className={`text-[10px] font-semibold ${status.textColor}`}>{status.label}</Text>
                </View>
                <Text className="text-[10px] text-zinc-500 mt-1">
                  {shortRelativeTime(session.updatedAt || session.createdAt)}
                </Text>
              </View>
            </TouchableOpacity>
          );
        }}
        ListEmptyComponent={
          <View className="items-center justify-center py-20">
            <Text className="text-zinc-500 text-sm">No sessions yet</Text>
          </View>
        }
      />

      {/* Search Bar */}
      <View className="absolute bottom-0 left-0 right-0 px-4 pb-4 pt-3 bg-[#0d0d0e]/95 border-t border-white/10">
        <View className="flex-row items-center bg-[#16171a] border border-white/10 rounded-xl px-4 h-12">
          <Search size={18} color="#71717a" />
          <TextInput
            className="flex-1 ml-3 text-white text-sm"
            placeholder="Search threads"
            placeholderTextColor="#71717a"
            value={searchQuery}
            onChangeText={setSearchQuery}
          />
          <TouchableOpacity className="p-2">
            <Plus size={20} color="#ffffff" />
          </TouchableOpacity>
        </View>
      </View>
    </View>
  );
}

export default HomeScreen;
